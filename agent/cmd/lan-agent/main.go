package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/kardianos/service"

	"github.com/mediacode/lan-commander/agent/internal/discovery"
	"github.com/mediacode/lan-commander/agent/internal/server"
	"github.com/mediacode/lan-commander/agent/internal/ui"
)

// Version is set at build time.
var Version = "1.0.0"

// agentFlags holds the parsed CLI configuration for a single run.
type agentFlags struct {
	port             int
	host             string
	name             string
	discoveryEnabled bool
	tlsCert          string
	tlsKey           string
	authToken        string
	noAuth           bool
	uiMode           bool
	managedByNotice  string
}

func parseAgentFlags(fs *flag.FlagSet, args []string) *agentFlags {
	f := &agentFlags{}
	fs.IntVar(&f.port, "port", 9474, "WebSocket server port")
	fs.StringVar(&f.host, "host", "0.0.0.0", "Bind address")
	fs.StringVar(&f.name, "name", "", "Agent name (default: hostname)")
	fs.BoolVar(&f.discoveryEnabled, "discovery", true, "Enable mDNS discovery")
	fs.StringVar(&f.tlsCert, "tls-cert", "", "TLS certificate file path")
	fs.StringVar(&f.tlsKey, "tls-key", "", "TLS private key file path")
	fs.StringVar(&f.authToken, "auth-token", "", "Authentication token for client connections")
	fs.BoolVar(&f.noAuth, "no-auth", false, "Disable authentication (only for an isolated laboratory network)")
	fs.BoolVar(&f.uiMode, "ui", false, "Run the client interface in the current desktop session")
	fs.StringVar(&f.managedByNotice, "managed-by-notice", "", "Organisation name shown in the managed-device notice")
	_ = fs.Parse(args)
	return f
}

func validateAgentFlags(f *agentFlags) error {
	if f.noAuth && f.authToken != "" {
		return fmt.Errorf("--no-auth cannot be combined with --auth-token")
	}
	if f.noAuth {
		return nil
	}
	if f.authToken == "" {
		return fmt.Errorf("authentication token is required: pass --auth-token <token>, or explicitly use --no-auth only on an isolated laboratory network")
	}
	return nil
}

// program implements service.Interface so the same binary can run in the
// foreground, or be installed/managed as a Windows Service or systemd unit.
type program struct {
	flags  *agentFlags
	cancel context.CancelFunc
}

func (p *program) Start(s service.Service) error {
	if err := validateAgentFlags(p.flags); err != nil {
		return err
	}

	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	go p.run(ctx)
	return nil
}

func (p *program) Stop(s service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	return nil
}

func (p *program) run(ctx context.Context) {
	f := p.flags

	name := f.name
	if name == "" {
		hostname, err := os.Hostname()
		if err != nil {
			log.Printf("[main] Cannot get hostname: %v", err)
			hostname = "unknown"
		}
		name = hostname
	}

	log.Print(banner(name, f.port))

	addr := fmt.Sprintf("%s:%d", f.host, f.port)
	srv := server.NewServer(addr, f.tlsCert, f.tlsKey, f.authToken)

	var mdnsService *discovery.MDNSService
	if f.discoveryEnabled {
		var err error
		mdnsService, err = discovery.NewMDNSService(f.port, f.authToken != "")
		if err != nil {
			log.Printf("[discovery] Failed to start mDNS: %v", err)
		} else {
			log.Printf("[discovery] Advertising service %s on port %d", discovery.ServiceName, f.port)
			defer mdnsService.Close()
		}
	}

	log.Printf("[main] LAN Commander Agent v%s starting on %s", Version, addr)
	if err := srv.Start(ctx); err != nil {
		log.Printf("[main] Server stopped: %v", err)
	}
	log.Println("[main] Shutdown complete")
}

func banner(name string, port int) string {
	return fmt.Sprintf(`
  _                    _____                      _
 | |                  / ____|                    | |
 | |      __ _  ___  | |     ___  _ __ ___  _ __ | | ___ _ __
 | |     / _  |/ __| | |    / _ \| '_   _ \| '_ \| |/ _ \ '__|
 | |____| (_| | (__  | |___| (_) | | | | | | |_) | |  __/ |
 |______|\__,_|\___|  \_____\___/|_| |_| |_| .__/|_|\___|_|
                                           | |
                                           |_|
 Agent: %s
 Port:  %d
 Version: %s
`, name, port, Version)
}

func main() {
	// The first argument, if one of the service verbs below, controls the
	// installed service instead of running the agent directly. Any flags
	// that follow (e.g. --port, --auth-token) are persisted as the
	// service's startup arguments so they survive reboots.
	serviceCmds := map[string]bool{"install": true, "uninstall": true, "start": true, "stop": true, "restart": true}
	var cmd string
	runArgs := os.Args[1:]
	if len(os.Args) > 1 && serviceCmds[os.Args[1]] {
		cmd = os.Args[1]
		runArgs = os.Args[2:]
	}

	fs := flag.NewFlagSet("lan-agent", flag.ExitOnError)
	f := parseAgentFlags(fs, runArgs)

	if f.uiMode {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := ui.Run(ctx, ui.Config{Port: f.port, ManagedByNotice: f.managedByNotice, AgentVersion: Version}); err != nil {
			log.Fatalf("[main] Interface failed: %v", err)
		}
		return
	}

	svcConfig := &service.Config{
		Name:        "LANCommanderAgent",
		DisplayName: "LAN Commander Agent",
		Description: "Remote management agent for the LAN Commander control center.",
		Arguments:   runArgs,
	}
	// Install as a system-wide service (not per-user) so it runs before login.
	svcConfig.Option = service.KeyValue{
		"UserService": false,
	}

	prg := &program{flags: f}
	s, err := service.New(prg, svcConfig)
	if err != nil {
		log.Fatalf("[main] Failed to init service: %v", err)
	}

	switch cmd {
	case "install":
		if err := s.Install(); err != nil {
			log.Fatalf("[main] Install failed: %v", err)
		}
		fmt.Println("Service installed. Start it with: lan-agent start")
		return
	case "uninstall":
		if err := s.Uninstall(); err != nil {
			log.Fatalf("[main] Uninstall failed: %v", err)
		}
		fmt.Println("Service uninstalled.")
		return
	case "start":
		if err := s.Start(); err != nil {
			log.Fatalf("[main] Start failed: %v", err)
		}
		fmt.Println("Service started.")
		return
	case "stop":
		if err := s.Stop(); err != nil {
			log.Fatalf("[main] Stop failed: %v", err)
		}
		fmt.Println("Service stopped.")
		return
	case "restart":
		if err := s.Restart(); err != nil {
			log.Fatalf("[main] Restart failed: %v", err)
		}
		fmt.Println("Service restarted.")
		return
	}

	// No service verb: run in the foreground (dev/manual use), or as the
	// managed process when launched by the OS service manager itself.
	if err := s.Run(); err != nil {
		log.Fatalf("[main] %v", err)
	}
}
