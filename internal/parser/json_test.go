package parser

import (
	"errors"
	"testing"
	"time"

	"github.com/kanechoo/scamper-client-go/internal/core"
)

const sampleTrace = `{"type":"trace","version":"0.1","userid":5,"method":"udp","src":"192.168.1.2","dst":"1.1.1.1","stop_reason":"COMPLETED","stop_data":0,"hop_count":2,"probe_count":4,"hops":[{"addr":"192.168.1.1","rtt":1.234,"reply_ttl":64,"probe_ttl":1,"probe_size":28},{"addr":"1.1.1.1","name":"one.one.one.one","rtt":15.309,"reply_ttl":57,"probe_ttl":2}]}`

func TestParseTrace(t *testing.T) {
	res, err := ParseTrace([]byte(sampleTrace), "1.1.1.1")
	if err != nil {
		t.Fatalf("ParseTrace() error = %v", err)
	}
	if res.Target != "1.1.1.1" {
		t.Errorf("Target = %q", res.Target)
	}
	if res.Address != "1.1.1.1" {
		t.Errorf("Address = %q", res.Address)
	}
	if res.Method != "udp" {
		t.Errorf("Method = %q", res.Method)
	}
	if res.StopReason != "COMPLETED" {
		t.Errorf("StopReason = %q", res.StopReason)
	}
	if res.Error != nil {
		t.Errorf("Error = %v", res.Error)
	}
	if len(res.Hops) != 2 {
		t.Fatalf("len(Hops) = %d, want 2", len(res.Hops))
	}
	h0 := res.Hops[0]
	if h0.TTL != 1 || h0.Address != "192.168.1.1" || h0.ReplyTTL != 64 || h0.ProbeSize != 28 {
		t.Errorf("hop0 = %+v", h0)
	}
	if h0.RTT != time.Duration(1.234*float64(time.Millisecond)) {
		t.Errorf("hop0.RTT = %v", h0.RTT)
	}
	if res.Hops[1].Name != "one.one.one.one" {
		t.Errorf("hop1.Name = %q", res.Hops[1].Name)
	}
	if res.Metadata == nil {
		t.Fatal("Metadata is nil")
	}
	if res.Metadata["probe_count"] != 4 {
		t.Errorf("Metadata[probe_count] = %v", res.Metadata["probe_count"])
	}
	if res.Metadata["stop_data"] != int64(0) {
		t.Errorf("Metadata[stop_data] = %v", res.Metadata["stop_data"])
	}
	if res.Metadata["src"] != "192.168.1.2" {
		t.Errorf("Metadata[src] = %v", res.Metadata["src"])
	}
}

func TestParseTraceSkipsNonTraceStreams(t *testing.T) {
	stream := `{"type":"cycle-start","start":{"sec":1,"usec":2}}` + "\n" +
		`{"type":"list","list_id":0}` + "\n" +
		sampleTrace + "\n" +
		`{"type":"cycle-stop"}` + "\n"
	res, err := ParseTrace([]byte(stream), "1.1.1.1")
	if err != nil {
		t.Fatalf("ParseTrace() error = %v", err)
	}
	if len(res.Hops) != 2 {
		t.Fatalf("len(Hops) = %d, want 2", len(res.Hops))
	}
}

func TestParseTraceNoTraceRecord(t *testing.T) {
	_, err := ParseTrace([]byte(`{"type":"cycle-stop"}`), "1.1.1.1")
	if !errors.Is(err, core.ErrParse) {
		t.Fatalf("ParseTrace() error = %v, want ErrParse", err)
	}
}

func TestParseTraceInvalidJSON(t *testing.T) {
	_, err := ParseTrace([]byte(`{not json`), "1.1.1.1")
	if !errors.Is(err, core.ErrParse) {
		t.Fatalf("ParseTrace() error = %v, want ErrParse", err)
	}
}

func TestParseTraceErrorMsg(t *testing.T) {
	data := `{"type":"trace","method":"udp","dst":"1.1.1.1","stop_reason":"ERROR","errmsg":"permission denied","hops":[]}`
	res, err := ParseTrace([]byte(data), "1.1.1.1")
	if err != nil {
		t.Fatalf("ParseTrace() error = %v", err)
	}
	if res.Error == nil {
		t.Fatal("Error is nil, want measurement error")
	}
	if res.Metadata["errmsg"] != "permission denied" {
		t.Errorf("Metadata[errmsg] = %v", res.Metadata["errmsg"])
	}
}

func TestParseTraceNoMetadata(t *testing.T) {
	data := `{"type":"trace","dst":"1.1.1.1","hops":[]}`
	res, err := ParseTrace([]byte(data), "1.1.1.1")
	if err != nil {
		t.Fatal(err)
	}
	if res.Metadata != nil {
		t.Errorf("Metadata = %v, want nil", res.Metadata)
	}
}
