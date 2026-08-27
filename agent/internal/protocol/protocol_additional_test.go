package protocol

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func protocolRoundTrip[T any](t *testing.T, want T) T {
	t.Helper()

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal %T: %v", want, err)
	}

	var got T
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal %T: %v", want, err)
	}
	return got
}

func protocolJSONFields(t *testing.T, value any) map[string]json.RawMessage {
	t.Helper()

	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal %T: %v", value, err)
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("decode JSON object for %T: %v", value, err)
	}
	return fields
}

func assertJSONFieldPresence(t *testing.T, fields map[string]json.RawMessage, present, absent []string) {
	t.Helper()

	for _, name := range present {
		if _, ok := fields[name]; !ok {
			t.Errorf("JSON field %q is missing", name)
		}
	}
	for _, name := range absent {
		if _, ok := fields[name]; ok {
			t.Errorf("JSON field %q is present but should be omitted", name)
		}
	}
}

func TestMessageEnvelopeRoundTrip(t *testing.T) {
	want := Message{
		ID:   "request-42",
		Type: MsgExecCommand,
		Payload: ExecCommandPayload{
			Command: "Get-ChildItem",
			Args:    []string{"-Force"},
			Timeout: 15,
			Shell:   "powershell",
		},
		Timestamp: time.Date(2026, time.August, 27, 15, 4, 5, 0, time.UTC),
	}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal message: %v", err)
	}

	var got Message
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal message: %v", err)
	}

	if got.ID != want.ID {
		t.Fatalf("message id = %q, want %q", got.ID, want.ID)
	}
	if got.Type != want.Type {
		t.Fatalf("message type = %q, want %q", got.Type, want.Type)
	}
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Fatalf("message timestamp = %v, want %v", got.Timestamp, want.Timestamp)
	}

	var gotPayload ExecCommandPayload
	payloadJSON, err := json.Marshal(got.Payload)
	if err != nil {
		t.Fatalf("remarshal decoded payload: %v", err)
	}
	if err := json.Unmarshal(payloadJSON, &gotPayload); err != nil {
		t.Fatalf("decode typed payload: %v", err)
	}
	if !reflect.DeepEqual(gotPayload, want.Payload) {
		t.Fatalf("decoded payload = %#v, want %#v", gotPayload, want.Payload)
	}

	fields := protocolJSONFields(t, want)
	assertJSONFieldPresence(t, fields, []string{"id", "type", "payload", "timestamp"}, []string{"error"})
}

func TestRequestPayloadRoundTrips(t *testing.T) {
	wantCommand := ExecCommandPayload{
		Command: "echo",
		Args:    []string{"hello", "wire protocol"},
		Timeout: 30,
		Shell:   "bash",
	}
	if got := protocolRoundTrip(t, wantCommand); !reflect.DeepEqual(got, wantCommand) {
		t.Fatalf("exec command payload = %#v, want %#v", got, wantCommand)
	}

	wantFile := SendFilePayload{
		Path:       "uploads/report.bin",
		Data:       []byte{0x00, 0x01, 0xfe, 0xff},
		Offset:     4,
		TotalSize:  12,
		Final:      true,
		Checksum:   "sha256-checksum",
		TransferID: "transfer-42",
	}
	if got := protocolRoundTrip(t, wantFile); !reflect.DeepEqual(got, wantFile) {
		t.Fatalf("send file payload = %#v, want %#v", got, wantFile)
	}

	wantCancel := CancelFilePayload{Path: "uploads/report.bin", TransferID: "transfer-42"}
	if got := protocolRoundTrip(t, wantCancel); !reflect.DeepEqual(got, wantCancel) {
		t.Fatalf("cancel file payload = %#v, want %#v", got, wantCancel)
	}
}

func TestResponsePayloadsRoundTrip(t *testing.T) {
	wantResult := CommandResultPayload{
		Stdout:   "ok\n",
		Stderr:   "",
		ExitCode: 0,
		Duration: 42,
	}
	if got := protocolRoundTrip(t, wantResult); !reflect.DeepEqual(got, wantResult) {
		t.Fatalf("command result payload = %#v, want %#v", got, wantResult)
	}

	wantChunk := FileChunkPayload{
		Path:      "downloads/report.bin",
		Data:      []byte{0x10, 0x20, 0x30},
		Offset:    8,
		TotalSize: 11,
		Final:     true,
		Checksum:  "sha256-checksum",
	}
	if got := protocolRoundTrip(t, wantChunk); !reflect.DeepEqual(got, wantChunk) {
		t.Fatalf("file chunk payload = %#v, want %#v", got, wantChunk)
	}

	wantSystem := SystemInfoPayload{
		Hostname: "agent-01",
		OS:       "windows",
		Platform: "windows",
		Arch:     "amd64",
		CPU: CPUInfo{
			Percent: 12.5,
			Cores:   8,
			Model:   "Example CPU",
		},
		Memory:       MemoryInfo{Total: 16, Used: 8, Free: 8, Percent: 50},
		Disks:        []DiskInfo{{Mount: "C:", FSType: "NTFS", Total: 100, Used: 25, Free: 75, Percent: 25}},
		Uptime:       99,
		Net:          NetInfo{IP: "192.0.2.10", MAC: "00:11:22:33:44:55", Hostname: "agent-01"},
		AgentVersion: "1.2.3",
	}
	if got := protocolRoundTrip(t, wantSystem); !reflect.DeepEqual(got, wantSystem) {
		t.Fatalf("system info payload = %#v, want %#v", got, wantSystem)
	}
}

func TestSnakeCaseJSONTags(t *testing.T) {
	getFileFields := protocolJSONFields(t, GetFilePayload{Path: "logs/app.log", ChunkSize: 4096})
	assertJSONFieldPresence(t, getFileFields, []string{"path", "chunk_size"}, []string{"ChunkSize"})

	resultFields := protocolJSONFields(t, CommandResultPayload{ExitCode: 7, Duration: 123})
	assertJSONFieldPresence(t, resultFields, []string{"stdout", "stderr", "exit_code", "duration_ms"}, []string{"ExitCode", "Duration"})

	sendFileFields := protocolJSONFields(t, SendFilePayload{Path: "data.bin", TotalSize: 10})
	assertJSONFieldPresence(t, sendFileFields, []string{"path", "total_size"}, []string{"TotalSize"})

	systemFields := protocolJSONFields(t, SystemInfoPayload{
		AgentVersion: "1.2.3",
		Disks:        []DiskInfo{{FSType: "NTFS"}},
	})
	assertJSONFieldPresence(t, systemFields, []string{"agent_version", "disks"}, []string{"AgentVersion"})

	var disks []map[string]json.RawMessage
	if err := json.Unmarshal(systemFields["disks"], &disks); err != nil {
		t.Fatalf("decode disks field: %v", err)
	}
	if len(disks) != 1 {
		t.Fatalf("decoded disks length = %d, want 1", len(disks))
	}
	assertJSONFieldPresence(t, disks[0], []string{"fs_type"}, []string{"FSType"})
}

func TestOmitEmptyFieldsWhereSpecified(t *testing.T) {
	messageFields := protocolJSONFields(t, Message{
		Type:      MsgKeepAlive,
		Payload:   nil,
		Timestamp: time.Date(2026, time.August, 27, 0, 0, 0, 0, time.UTC),
	})
	assertJSONFieldPresence(t, messageFields, []string{"type", "payload", "timestamp"}, []string{"id", "error"})

	commandFields := protocolJSONFields(t, ExecCommandPayload{Command: "pwd"})
	assertJSONFieldPresence(t, commandFields, []string{"command"}, []string{"args", "timeout", "shell"})

	sendFileFields := protocolJSONFields(t, SendFilePayload{Path: "data.bin"})
	assertJSONFieldPresence(t, sendFileFields, []string{"path"}, []string{"data", "offset", "total_size", "final", "checksum", "transfer_id"})

	chunkFields := protocolJSONFields(t, FileChunkPayload{Path: "data.bin", Data: nil, Offset: 0, Final: false})
	assertJSONFieldPresence(t, chunkFields, []string{"path", "data", "offset", "final"}, []string{"total_size", "checksum"})
}

type protocolFieldContract struct {
	name    string
	jsonTag string
}

// This canonical wire contract is intentionally duplicated in the other
// module's protocol tests because the two protocol packages cannot import one
// another. Any drift in either local definition should fail its package tests.
var expectedProtocolFieldContracts = map[string][]protocolFieldContract{
	"Message": {
		{name: "ID", jsonTag: "id,omitempty"},
		{name: "Type", jsonTag: "type"},
		{name: "Payload", jsonTag: "payload"},
		{name: "Timestamp", jsonTag: "timestamp"},
		{name: "Error", jsonTag: "error,omitempty"},
	},
	"ExecCommandPayload": {
		{name: "Command", jsonTag: "command"},
		{name: "Args", jsonTag: "args,omitempty"},
		{name: "Timeout", jsonTag: "timeout,omitempty"},
		{name: "Shell", jsonTag: "shell,omitempty"},
	},
	"ListDirPayload": {{name: "Path", jsonTag: "path"}},
	"GetFilePayload": {
		{name: "Path", jsonTag: "path"},
		{name: "Offset", jsonTag: "offset,omitempty"},
		{name: "ChunkSize", jsonTag: "chunk_size,omitempty"},
	},
	"SendFilePayload": {
		{name: "Path", jsonTag: "path"},
		{name: "Data", jsonTag: "data,omitempty"},
		{name: "Offset", jsonTag: "offset,omitempty"},
		{name: "TotalSize", jsonTag: "total_size,omitempty"},
		{name: "Final", jsonTag: "final,omitempty"},
		{name: "Checksum", jsonTag: "checksum,omitempty"},
		{name: "TransferID", jsonTag: "transfer_id,omitempty"},
	},
	"CancelFilePayload": {
		{name: "Path", jsonTag: "path"},
		{name: "TransferID", jsonTag: "transfer_id"},
	},
	"AuthPayload": {
		{name: "Token", jsonTag: "token"},
		{name: "Username", jsonTag: "username,omitempty"},
	},
	"ScriptRunPayload": {
		{name: "Name", jsonTag: "name"},
		{name: "Script", jsonTag: "script"},
		{name: "Args", jsonTag: "args,omitempty"},
	},
	"CommandResultPayload": {
		{name: "Stdout", jsonTag: "stdout"},
		{name: "Stderr", jsonTag: "stderr"},
		{name: "ExitCode", jsonTag: "exit_code"},
		{name: "Duration", jsonTag: "duration_ms"},
	},
	"DirEntry": {
		{name: "Name", jsonTag: "name"},
		{name: "Path", jsonTag: "path"},
		{name: "IsDir", jsonTag: "is_dir"},
		{name: "Size", jsonTag: "size"},
		{name: "Mode", jsonTag: "mode"},
		{name: "ModTime", jsonTag: "mod_time"},
	},
	"DirContentsPayload": {
		{name: "Path", jsonTag: "path"},
		{name: "Entries", jsonTag: "entries"},
		{name: "Total", jsonTag: "total"},
	},
	"FileChunkPayload": {
		{name: "Path", jsonTag: "path"},
		{name: "Data", jsonTag: "data"},
		{name: "Offset", jsonTag: "offset"},
		{name: "TotalSize", jsonTag: "total_size,omitempty"},
		{name: "Final", jsonTag: "final"},
		{name: "Checksum", jsonTag: "checksum,omitempty"},
	},
	"SystemInfoPayload": {
		{name: "Hostname", jsonTag: "hostname"},
		{name: "OS", jsonTag: "os"},
		{name: "Platform", jsonTag: "platform"},
		{name: "Arch", jsonTag: "arch"},
		{name: "CPU", jsonTag: "cpu"},
		{name: "Memory", jsonTag: "memory"},
		{name: "Disks", jsonTag: "disks"},
		{name: "Uptime", jsonTag: "uptime"},
		{name: "Net", jsonTag: "net"},
		{name: "AgentVersion", jsonTag: "agent_version"},
	},
	"CPUInfo": {
		{name: "Percent", jsonTag: "percent"},
		{name: "Cores", jsonTag: "cores"},
		{name: "Model", jsonTag: "model"},
	},
	"MemoryInfo": {
		{name: "Total", jsonTag: "total"},
		{name: "Used", jsonTag: "used"},
		{name: "Free", jsonTag: "free"},
		{name: "Percent", jsonTag: "percent"},
	},
	"DiskInfo": {
		{name: "Mount", jsonTag: "mount"},
		{name: "FSType", jsonTag: "fs_type"},
		{name: "Total", jsonTag: "total"},
		{name: "Used", jsonTag: "used"},
		{name: "Free", jsonTag: "free"},
		{name: "Percent", jsonTag: "percent"},
	},
	"NetInfo": {
		{name: "IP", jsonTag: "ip"},
		{name: "MAC", jsonTag: "mac"},
		{name: "Hostname", jsonTag: "hostname"},
	},
	"ScreenshotDataPayload": {
		{name: "Format", jsonTag: "format"},
		{name: "Data", jsonTag: "data"},
		{name: "Width", jsonTag: "width"},
		{name: "Height", jsonTag: "height"},
	},
	"AgentInfoPayload": {
		{name: "Hostname", jsonTag: "hostname"},
		{name: "OS", jsonTag: "os"},
		{name: "Arch", jsonTag: "arch"},
		{name: "AgentVersion", jsonTag: "agent_version"},
		{name: "Port", jsonTag: "port"},
		{name: "Authenticated", jsonTag: "authenticated"},
	},
}

func TestCriticalPayloadFieldsMatchSharedWireContract(t *testing.T) {
	actualTypes := map[string]reflect.Type{
		"Message":               reflect.TypeOf(Message{}),
		"ExecCommandPayload":    reflect.TypeOf(ExecCommandPayload{}),
		"ListDirPayload":        reflect.TypeOf(ListDirPayload{}),
		"GetFilePayload":        reflect.TypeOf(GetFilePayload{}),
		"SendFilePayload":       reflect.TypeOf(SendFilePayload{}),
		"CancelFilePayload":     reflect.TypeOf(CancelFilePayload{}),
		"AuthPayload":           reflect.TypeOf(AuthPayload{}),
		"ScriptRunPayload":      reflect.TypeOf(ScriptRunPayload{}),
		"CommandResultPayload":  reflect.TypeOf(CommandResultPayload{}),
		"DirEntry":              reflect.TypeOf(DirEntry{}),
		"DirContentsPayload":    reflect.TypeOf(DirContentsPayload{}),
		"FileChunkPayload":      reflect.TypeOf(FileChunkPayload{}),
		"SystemInfoPayload":     reflect.TypeOf(SystemInfoPayload{}),
		"CPUInfo":               reflect.TypeOf(CPUInfo{}),
		"MemoryInfo":            reflect.TypeOf(MemoryInfo{}),
		"DiskInfo":              reflect.TypeOf(DiskInfo{}),
		"NetInfo":               reflect.TypeOf(NetInfo{}),
		"ScreenshotDataPayload": reflect.TypeOf(ScreenshotDataPayload{}),
		"AgentInfoPayload":      reflect.TypeOf(AgentInfoPayload{}),
	}

	if len(actualTypes) != len(expectedProtocolFieldContracts) {
		t.Errorf("protocol type count = %d, want %d", len(actualTypes), len(expectedProtocolFieldContracts))
	}

	for typeName, wantFields := range expectedProtocolFieldContracts {
		actual, ok := actualTypes[typeName]
		if !ok {
			t.Errorf("missing protocol type %q", typeName)
			continue
		}
		if actual.NumField() != len(wantFields) {
			t.Errorf("%s field count = %d, want %d", typeName, actual.NumField(), len(wantFields))
		}
		for _, want := range wantFields {
			field, ok := actual.FieldByName(want.name)
			if !ok {
				t.Errorf("%s is missing field %s", typeName, want.name)
				continue
			}
			if got := field.Tag.Get("json"); got != want.jsonTag {
				t.Errorf("%s.%s JSON tag = %q, want %q", typeName, want.name, got, want.jsonTag)
			}
		}
	}

	for typeName := range actualTypes {
		if _, ok := expectedProtocolFieldContracts[typeName]; !ok {
			t.Errorf("unexpected protocol type %q", typeName)
		}
	}
}

func TestMessageConstantsMatchSharedWireContract(t *testing.T) {
	want := map[string]string{
		"MsgExecCommand":    "exec_command",
		"MsgListDir":        "list_dir",
		"MsgGetFile":        "get_file",
		"MsgSendFile":       "send_file",
		"MsgCancelFile":     "cancel_file",
		"MsgScreenshot":     "screenshot",
		"MsgSystemInfo":     "system_info",
		"MsgAuth":           "auth",
		"MsgKeepAlive":      "keep_alive",
		"MsgScriptRun":      "script_run",
		"MsgCommandResult":  "command_result",
		"MsgDirContents":    "dir_contents",
		"MsgFileChunk":      "file_chunk",
		"MsgFileAck":        "file_ack",
		"MsgScreenshotData": "screenshot_data",
		"MsgSystemUpdate":   "system_update",
		"MsgError":          "error",
		"MsgAuthRequired":   "auth_required",
		"MsgAuthOk":         "auth_ok",
		"MsgAgentInfo":      "agent_info",
	}
	got := map[string]string{
		"MsgExecCommand":    MsgExecCommand,
		"MsgListDir":        MsgListDir,
		"MsgGetFile":        MsgGetFile,
		"MsgSendFile":       MsgSendFile,
		"MsgCancelFile":     MsgCancelFile,
		"MsgScreenshot":     MsgScreenshot,
		"MsgSystemInfo":     MsgSystemInfo,
		"MsgAuth":           MsgAuth,
		"MsgKeepAlive":      MsgKeepAlive,
		"MsgScriptRun":      MsgScriptRun,
		"MsgCommandResult":  MsgCommandResult,
		"MsgDirContents":    MsgDirContents,
		"MsgFileChunk":      MsgFileChunk,
		"MsgFileAck":        MsgFileAck,
		"MsgScreenshotData": MsgScreenshotData,
		"MsgSystemUpdate":   MsgSystemUpdate,
		"MsgError":          MsgError,
		"MsgAuthRequired":   MsgAuthRequired,
		"MsgAuthOk":         MsgAuthOk,
		"MsgAgentInfo":      MsgAgentInfo,
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("message constants = %#v, want %#v", got, want)
	}
}
