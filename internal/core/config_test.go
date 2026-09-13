package core

import (
	"errors"
	"reflect"
	"testing"
	"time"
)

func TestBuildEmptyHasNoArgs(t *testing.T) {
	args, err := NewTraceConfig().Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("Build() = %v, want empty", args)
	}
}

func TestBuildFull(t *testing.T) {
	cfg := NewTraceConfig().
		Protocol(ProtocolUDP).
		FirstHop(9).MaxHops(16).
		Wait(time.Second).ProbesPerHop(1).
		Source("192.168.6.254").
		DstPort(33434).SrcPort(0).
		TOS(0).
		Confidence(Confidence99).
		GapLimit(12).GapAction(GapActionHalt).
		WaitProbe(20 * time.Millisecond).
		WaitProbeHop(time.Second).
		Loops(1).Squeries(1).
		Offset(0).
		PayloadHex("00").
		UserID(7).
		Stream(0).
		PMTUD(true).
		AllProbes(false).
		TTLExceededNotDest(true).
		Option(OptionPTR).
		Option(OptionBack)

	got, err := cfg.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	want := []string{
		"-P", "udp",
		"-f", "9",
		"-m", "16",
		"-w", "1",
		"-q", "1",
		"-S", "192.168.6.254",
		"-d", "33434",
		"-s", "0",
		"-t", "0",
		"-c", "99",
		"-g", "12",
		"-G", "1",
		"-W", "2",
		"-H", "1",
		"-l", "1",
		"-N", "1",
		"-o", "0",
		"-p", "00",
		"-U", "7",
		"-y", "0",
		"-M",
		"-T",
		"-O", "ptr",
		"-O", "back",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() =\n %v\nwant\n %v", got, want)
	}
}

func TestBuildOnlySetOptions(t *testing.T) {
	got, err := NewTraceConfig().Protocol(ProtocolTCPAck).MaxHops(5).Build()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-P", "tcp-ack", "-m", "5"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() = %v, want %v", got, want)
	}
}

func TestOptionDeduplication(t *testing.T) {
	got, err := NewTraceConfig().
		Option(OptionPTR).Option(OptionBack).Option(OptionPTR).
		Build()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-O", "ptr", "-O", "back"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() = %v, want %v", got, want)
	}
}

func TestExtraArgsAppendedLast(t *testing.T) {
	got, err := NewTraceConfig().
		Protocol(ProtocolUDP).
		ExtraArgs("-z", "10.0.0.1").
		Build()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-P", "udp", "-z", "10.0.0.1"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Build() = %v, want %v", got, want)
	}
}

func TestCloneIsIndependent(t *testing.T) {
	base := NewTraceConfig().Protocol(ProtocolUDP).ExtraArgs("-z")
	clone := base.Clone()
	clone.MaxHops(10)
	clone.Option(OptionPTR)

	baseArgs, err := base.Build()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(baseArgs, []string{"-P", "udp", "-z"}) {
		t.Fatalf("base mutated: %v", baseArgs)
	}
	cloneArgs, err := clone.Build()
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-P", "udp", "-m", "10", "-O", "ptr", "-z"}
	if !reflect.DeepEqual(cloneArgs, want) {
		t.Fatalf("clone = %v, want %v", cloneArgs, want)
	}
}

func TestValidateRejectsInvalid(t *testing.T) {
	cases := []struct {
		name string
		cfg  *TraceConfig
	}{
		{"bad method", NewTraceConfig().Protocol(Method("grep"))},
		{"firsthop 0", NewTraceConfig().FirstHop(0)},
		{"maxhops 256", NewTraceConfig().MaxHops(256)},
		{"first>max", NewTraceConfig().FirstHop(10).MaxHops(5)},
		{"negative wait", NewTraceConfig().Wait(-time.Second)},
		{"probes 0", NewTraceConfig().ProbesPerHop(0)},
		{"bad source", NewTraceConfig().Source("not-an-ip")},
		{"dport high", NewTraceConfig().DstPort(70000)},
		{"tos high", NewTraceConfig().TOS(256)},
		{"bad confidence", NewTraceConfig().Confidence(Confidence(90))},
		{"bad gapaction", NewTraceConfig().GapAction(GapAction(3))},
		{"waithop too big", NewTraceConfig().WaitProbeHop(3 * time.Second)},
		{"squeries 0", NewTraceConfig().Squeries(0)},
		{"squeries>=gap", NewTraceConfig().GapLimit(5).Squeries(5)},
		{"offset high", NewTraceConfig().Offset(8192)},
		{"odd payload", NewTraceConfig().PayloadHex("0")},
		{"bad payload", NewTraceConfig().PayloadHex("zz")},
		{"bad router", NewTraceConfig().RouterAddr("nope")},
		{"stream negative", NewTraceConfig().Stream(-1)},
		{"bad option", NewTraceConfig().Option(Option("nope"))},
		{"extra with space", NewTraceConfig().ExtraArgs("a b")},
		{"extra with newline", NewTraceConfig().ExtraArgs("a\nb")},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.cfg.Validate()
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Validate() = %v, want ErrInvalidConfig", err)
			}
		})
	}
}

func TestValidateEmptySourceRouterMeansUnset(t *testing.T) {
	cfg := NewTraceConfig().Source("").RouterAddr("").PayloadHex("")
	args, err := cfg.Build()
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if len(args) != 0 {
		t.Fatalf("Build() = %v, want empty", args)
	}
}

func TestBuildArgv(t *testing.T) {
	argv, err := BuildArgv(NewTraceConfig().Protocol(ProtocolICMP), "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"trace", "-P", "icmp", "1.1.1.1"}
	if !reflect.DeepEqual(argv, want) {
		t.Fatalf("BuildArgv() = %v, want %v", argv, want)
	}
}

func TestValidateToken(t *testing.T) {
	valid := []string{"1.1.1.1", "www.example.com", "2001:db8::1", "-z"}
	for _, s := range valid {
		if err := ValidateToken(s); err != nil {
			t.Errorf("ValidateToken(%q) = %v, want nil", s, err)
		}
	}
	invalid := []string{"", "a b", "a\tb", "a\nb", "a\rb", "a\x00b"}
	for _, s := range invalid {
		if err := ValidateToken(s); err == nil {
			t.Errorf("ValidateToken(%q) = nil, want error", s)
		}
	}
}

func TestReconnectPolicyNormalizeAndNext(t *testing.T) {
	p := ReconnectPolicy{}.Normalized()
	if p.MinBackoff != time.Second || p.MaxBackoff != 30*time.Second || p.Multiplier != 2 {
		t.Fatalf("Normalized() = %+v", p)
	}
	next := p.Next(p.MinBackoff)
	if next != 2*time.Second {
		t.Fatalf("Next(1s) = %v, want 2s", next)
	}
	capped := p.Next(20 * time.Second)
	if capped != 30*time.Second {
		t.Fatalf("Next(20s) = %v, want 30s", capped)
	}
}

func TestReconnectPolicyJitterBounds(t *testing.T) {
	p := ReconnectPolicy{MinBackoff: time.Second, MaxBackoff: time.Minute, Multiplier: 2, Jitter: true}
	for i := 0; i < 200; i++ {
		got := p.Next(time.Second)
		// equal jitter：落在 [next/2, next) = [1s, 2s)。
		if got < time.Second || got >= 2*time.Second {
			t.Fatalf("Next(1s) with jitter = %v, want in [1s, 2s)", got)
		}
	}
}
