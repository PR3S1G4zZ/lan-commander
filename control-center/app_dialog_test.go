package main

import "testing"

func TestTransferDialogBindingsValidateArgumentsBeforeOpeningDialogs(t *testing.T) {
	app := &App{}

	if err := app.DownloadFile("", "/remote/file"); err == nil {
		t.Fatal("DownloadFile accepted an empty agent ID")
	}
	if err := app.DownloadFile("agent-1", ""); err == nil {
		t.Fatal("DownloadFile accepted an empty remote path")
	}
	if err := app.UploadFile("", "/remote"); err == nil {
		t.Fatal("UploadFile accepted an empty agent ID")
	}
	if err := app.UploadFile("agent-1", ""); err == nil {
		t.Fatal("UploadFile accepted an empty remote directory")
	}
}
