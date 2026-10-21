package vtui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/unxed/vtinput"
)

func TestProtocol_Lifecycle(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(NewDesktop())
	defer fm.Shutdown()
	fm.SetHostMode(true)
	runDone := make(chan struct{})
	go func() {
		fm.Run()
		close(runDone)
	}()
	waitForCondition(t, time.Second, func() bool { return fm.running.Load() })

	oldFM := FrameManager
	FrameManager = fm
	defer func() { FrameManager = oldFM }()

	clientReader, serverWriter := io.Pipe()
	serverReader, clientWriter := io.Pipe()

	session := NewProtocolSession(serverReader, serverWriter, fm)

	serveDone := make(chan struct{})
	go func() {
		_ = session.Serve()
		close(serveDone)
	}()
	decoder := json.NewDecoder(clientReader)
	readReply := func(seq int) UpMessage {
		t.Helper()
		for {
			var msg UpMessage
			if err := decoder.Decode(&msg); err != nil {
				t.Fatalf("Failed to decode reply %d: %v", seq, err)
			}
			if msg.Op == "error" {
				t.Fatalf("protocol error before reply %d: %s: %s", seq, msg.Code, msg.Message)
			}
			if msg.ReplyTo == seq {
				return msg
			}
		}
	}
	barrier := func(seq int) {
		t.Helper()
		writeDone := make(chan struct{})
		go func() {
			_, _ = clientWriter.Write([]byte(fmt.Sprintf("{\"op\":\"hello\",\"seq\":%d,\"version\":1}\n", seq)))
			close(writeDone)
		}()
		_ = readReply(seq)
		<-writeDone
	}

	// 1. Send "hello"
	helloMsg := `{"op":"hello","seq":1,"version":1}` + "\n"
	_, _ = clientWriter.Write([]byte(helloMsg))

	welcome := readReply(1)

	if welcome.Op != "welcome" || welcome.ReplyTo != 1 || welcome.Version != 1 {
		t.Errorf("Unexpected welcome payload: %+v", welcome)
	}

	// 2. Send "mount" with dialog (JSON Lines formatted as single line)
	mountMsg := `{"op":"mount","frameId":"testDlg","tree":{"type":"Dialog","id":"testDlg","props":{"title":" Test Dialog "},"children":[{"type":"Edit","id":"userEdit","props":{"text":"Alice"}},{"type":"Button","id":"submitBtn","props":{"text":"&Ok","command":1000}}]}}` + "\n"

	_, _ = clientWriter.Write([]byte(mountMsg))
	barrier(2)

	var edit *Edit
	if err := fm.callOnUI(func() error {
		el, ok := fm.Lookup("testDlg", "userEdit")
		if !ok {
			return fmt.Errorf("userEdit not mounted")
		}
		edit = el.(*Edit)
		if got := edit.GetText(); got != "Alice" {
			return fmt.Errorf("expected edit text Alice, got %q", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 3. Send "patch" to update Edit text (JSON Lines single line)
	patchMsg := `{"op":"patch","frameId":"testDlg","ops":[{"kind":"set","id":"userEdit","props":{"text":"Bob"}}]}` + "\n"

	_, _ = clientWriter.Write([]byte(patchMsg))
	barrier(3)

	if err := fm.callOnUI(func() error {
		if got := edit.GetText(); got != "Bob" {
			return fmt.Errorf("expected patched text Bob, got %q", got)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 4. Protocol calls can focus an element and select all of an Edit's text.
	if err := fm.callOnUI(func() error {
		if err := session.handleCall(&DownMessage{ID: "userEdit", Method: "selectAll"}); err != nil {
			return err
		}
		if edit.selStart != 0 || edit.selEnd != len(edit.text) || !edit.clearFlag {
			return fmt.Errorf("selectAll state = (%d, %d, clear=%t), want whole text", edit.selStart, edit.selEnd, edit.clearFlag)
		}
		if err := session.handleCall(&DownMessage{ID: "userEdit", Method: "focus"}); err != nil {
			return err
		}
		if !edit.IsFocused() {
			return fmt.Errorf("focus call did not focus the edit")
		}
		if err := session.handleCall(&DownMessage{ID: "missing", Method: "focus"}); err == nil {
			return fmt.Errorf("call for missing element succeeded")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// 4. Trigger button command and verify event upstream
	if err := fm.callOnUI(func() error {
		btn, ok := fm.Lookup("testDlg", "submitBtn")
		if !ok {
			return fmt.Errorf("submitBtn not found")
		}
		btn.(*Button).ProcessKey(&vtinput.InputEvent{
			Type:           vtinput.KeyEventType,
			KeyDown:        true,
			VirtualKeyCode: vtinput.VK_RETURN,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	var cmdEvent UpMessage
	for {
		var ev UpMessage
		if err := decoder.Decode(&ev); err != nil {
			t.Fatalf("Failed to decode event: %v", err)
		}
		if ev.Op == "command" {
			cmdEvent = ev
			break
		}
	}

	if cmdEvent.Op != "command" || cmdEvent.Cmd != 1000 || cmdEvent.SrcID != "submitBtn" {
		t.Errorf("Unexpected command event: %+v", cmdEvent)
	}

	// 5. Send "quit"
	quitMsg := `{"op":"quit"}` + "\n"
	_, _ = clientWriter.Write([]byte(quitMsg))
	select {
	case <-serveDone:
	case <-time.After(time.Second):
		t.Fatal("protocol session did not stop after quit")
	}
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("frame manager did not stop after quit")
	}

	if !fm.IsShutdown() {
		t.Error("Expected FrameManager to be shutdown after quit")
	}

	_ = clientWriter.Close()
	_ = serverWriter.Close()
}

func TestProtocol_QuitBeforeRunDoesNotDeadlock(t *testing.T) {
	fm := &frameManager{}
	session := &ProtocolSession{fm: fm}
	done := make(chan error, 1)

	go func() {
		done <- session.handleMessage(&DownMessage{Op: "quit"})
	}()

	select {
	case err := <-done:
		if err != io.EOF {
			t.Fatalf("quit returned %v, want io.EOF", err)
		}
	case <-time.After(time.Second):
		t.Fatal("quit deadlocked before frame manager Run")
	}
}

func TestProtocol_HandlerValidationAndDescribe(t *testing.T) {
	var out bytes.Buffer
	ps := &ProtocolSession{out: &out}
	if err := ps.handleMessage(&DownMessage{Op: "mount"}); err == nil || !strings.Contains(err.Error(), "missing tree") {
		t.Fatalf("mount without tree error = %v", err)
	}
	if err := ps.handleMessage(&DownMessage{Op: "unknown"}); err == nil || !strings.Contains(err.Error(), "unknown operation") {
		t.Fatalf("unknown operation error = %v", err)
	}
	if err := ps.handleMessage(&DownMessage{Op: "hello", Seq: 7}); err != nil {
		t.Fatalf("hello: %v", err)
	}
	if err := ps.handleMessage(&DownMessage{Op: "describe", Seq: 8}); err != nil {
		t.Fatalf("describe: %v", err)
	}
	decoder := json.NewDecoder(&out)
	var welcome, description UpMessage
	if err := decoder.Decode(&welcome); err != nil || welcome.Op != "welcome" || welcome.ReplyTo != 7 {
		t.Fatalf("welcome = %#v, err %v", welcome, err)
	}
	if err := decoder.Decode(&description); err != nil || description.Op != "description" || description.ReplyTo != 8 || description.Value == nil {
		t.Fatalf("description = %#v, err %v", description, err)
	}
}

type protocolErrorWriter struct{ err error }

func (w protocolErrorWriter) Write([]byte) (int, error) { return 0, w.err }

func TestProtocol_SendErrorsAndClosedSession(t *testing.T) {
	t.Run("marshal error", func(t *testing.T) {
		ps := &ProtocolSession{out: &bytes.Buffer{}}
		if err := ps.send(UpMessage{Op: "event", Value: make(chan int)}); err == nil {
			t.Fatal("send accepted a value that cannot be encoded as JSON")
		}
	})

	t.Run("writer error", func(t *testing.T) {
		want := fmt.Errorf("output failed")
		ps := &ProtocolSession{out: protocolErrorWriter{err: want}}
		if err := ps.send(UpMessage{Op: "event"}); err != want {
			t.Fatalf("send error = %v, want %v", err, want)
		}
	})

	t.Run("closed session", func(t *testing.T) {
		ps := &ProtocolSession{out: &bytes.Buffer{}, closeDone: make(chan struct{})}
		ps.Close()
		if err := ps.send(UpMessage{Op: "event"}); err != io.ErrClosedPipe {
			t.Fatalf("send after Close error = %v, want %v", err, io.ErrClosedPipe)
		}
	})
}

func TestProtocol_PipeClosureTeardown(t *testing.T) {
	SetDefaultPalette()
	scr := NewSilentScreenBuf()
	scr.AllocBuf(80, 25)

	fm := &frameManager{}
	fm.Init(scr)
	fm.Push(NewDesktop())
	fm.SetHostMode(true)
	runDone := make(chan struct{})
	go func() {
		fm.Run()
		close(runDone)
	}()
	waitForCondition(t, time.Second, func() bool { return fm.running.Load() })

	var inBuf, outBuf bytes.Buffer
	inReader, inWriter := io.Pipe()
	session := NewProtocolSession(inReader, &outBuf, fm)

	serveDone := make(chan struct{})
	go func() {
		_ = session.Serve()
		close(serveDone)
	}()

	// Closing the pipe simulates child/host crash
	_ = inWriter.Close()
	select {
	case <-serveDone:
	case <-time.After(time.Second):
		t.Fatal("protocol session did not stop after pipe closure")
	}
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("frame manager did not stop after pipe closure")
	}

	if !fm.IsShutdown() {
		t.Error("Session should shutdown FrameManager on pipe closure")
	}
	_ = inBuf
}
