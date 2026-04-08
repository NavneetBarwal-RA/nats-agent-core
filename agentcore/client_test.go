package agentcore

import "testing"

// TestNewCreatesClientWithInitialState verifies that New(...) successfully
// constructs a client from the provided config, initializes the client health
// to StateNew, and preserves the configured public settings exactly as passed in.
//
// In Phase 1, New(...) does not yet perform full transport/session setup.
// So this test focuses only on constructor behavior that is actually implemented:
//   - client creation succeeds
//   - no error is returned
//   - initial health state is StateNew
//   - Config() returns the same config values that were provided to New(...)
// How To Run -
//     go test -v ./agentcore -run TestNewCreatesClientWithInitialState
// Results -
//     === RUN   TestNewCreatesClientWithInitialState
//     --- PASS: TestNewCreatesClientWithInitialState (0.00s)
//     PASS
//     ok  	github.com/routerarchitects/nats-agent-core/agentcore	0.002s

func TestNewCreatesClientWithInitialState(t *testing.T) {
	cfg := Config{
		AgentName: "test-agent",
		Version:   "1.0",
		NATS: NATSConfig{
			Servers: []string{"nats://localhost:4222"},
		},
		JetStream: JetStreamConfig{},
		Subjects: SubjectConfig{
			ConfigurePattern: "cmd.configure.%s",
			ActionPattern:    "cmd.action.%s.%s",
			ResultPattern:    "result.%s",
			StatusPattern:    "status.%s",
			HealthPattern:    "health.%s",
		},
		KV: KVConfig{
			Bucket:     "cfg_desired",
			KeyPattern: "desired.%s",
		},
	}

	client, err := New(cfg)
	if err != nil {
		t.Fatalf("New returned unexpected error: %v", err)
	}
	if client == nil {
		t.Fatal("New returned nil client")
	}

	health := client.Health()
	if health.State != StateNew {
		t.Fatalf("expected initial health state %q, got %q", StateNew, health.State)
	}

	gotCfg := client.Config()

	if gotCfg.AgentName != cfg.AgentName {
		t.Fatalf("expected AgentName %q, got %q", cfg.AgentName, gotCfg.AgentName)
	}
	if gotCfg.Version != cfg.Version {
		t.Fatalf("expected Version %q, got %q", cfg.Version, gotCfg.Version)
	}

	if len(gotCfg.NATS.Servers) != len(cfg.NATS.Servers) {
		t.Fatalf("expected %d NATS servers, got %d", len(cfg.NATS.Servers), len(gotCfg.NATS.Servers))
	}
	for i := range cfg.NATS.Servers {
		if gotCfg.NATS.Servers[i] != cfg.NATS.Servers[i] {
			t.Fatalf("expected NATS server %q at index %d, got %q", cfg.NATS.Servers[i], i, gotCfg.NATS.Servers[i])
		}
	}

	if gotCfg.JetStream.Domain != cfg.JetStream.Domain {
		t.Fatalf("expected JetStream Domain %q, got %q", cfg.JetStream.Domain, gotCfg.JetStream.Domain)
	}
	if gotCfg.JetStream.APIPrefix != cfg.JetStream.APIPrefix {
		t.Fatalf("expected JetStream APIPrefix %q, got %q", cfg.JetStream.APIPrefix, gotCfg.JetStream.APIPrefix)
	}
	if gotCfg.JetStream.DefaultTimeout != cfg.JetStream.DefaultTimeout {
		t.Fatalf("expected JetStream DefaultTimeout %v, got %v", cfg.JetStream.DefaultTimeout, gotCfg.JetStream.DefaultTimeout)
	}

	if gotCfg.Subjects.ConfigurePattern != cfg.Subjects.ConfigurePattern {
		t.Fatalf("expected ConfigurePattern %q, got %q", cfg.Subjects.ConfigurePattern, gotCfg.Subjects.ConfigurePattern)
	}
	if gotCfg.Subjects.ActionPattern != cfg.Subjects.ActionPattern {
		t.Fatalf("expected ActionPattern %q, got %q", cfg.Subjects.ActionPattern, gotCfg.Subjects.ActionPattern)
	}
	if gotCfg.Subjects.ResultPattern != cfg.Subjects.ResultPattern {
		t.Fatalf("expected ResultPattern %q, got %q", cfg.Subjects.ResultPattern, gotCfg.Subjects.ResultPattern)
	}
	if gotCfg.Subjects.StatusPattern != cfg.Subjects.StatusPattern {
		t.Fatalf("expected StatusPattern %q, got %q", cfg.Subjects.StatusPattern, gotCfg.Subjects.StatusPattern)
	}
	if gotCfg.Subjects.HealthPattern != cfg.Subjects.HealthPattern {
		t.Fatalf("expected HealthPattern %q, got %q", cfg.Subjects.HealthPattern, gotCfg.Subjects.HealthPattern)
	}

	if gotCfg.KV.Bucket != cfg.KV.Bucket {
		t.Fatalf("expected KV Bucket %q, got %q", cfg.KV.Bucket, gotCfg.KV.Bucket)
	}
	if gotCfg.KV.KeyPattern != cfg.KV.KeyPattern {
		t.Fatalf("expected KV KeyPattern %q, got %q", cfg.KV.KeyPattern, gotCfg.KV.KeyPattern)
	}
}
