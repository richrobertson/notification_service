package queue

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeRedisServer struct {
	ln    net.Listener
	mu    sync.Mutex
	lists map[string][]string
}

func newFakeRedisServer(t *testing.T) *fakeRedisServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeRedisServer{ln: ln, lists: map[string][]string{}}
	go s.serve(t)
	return s
}

func (s *fakeRedisServer) addr() string { return s.ln.Addr().String() }
func (s *fakeRedisServer) close()       { _ = s.ln.Close() }

func (s *fakeRedisServer) serve(t *testing.T) {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handleConn(t, conn)
	}
}

func (s *fakeRedisServer) handleConn(t *testing.T, conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	var tx [][]string
	for {
		cmd, err := readCommand(r)
		if err != nil {
			return
		}
		upper := strings.ToUpper(cmd[0])
		if len(tx) > 0 && upper != "EXEC" {
			tx = append(tx, cmd)
			writeSimple(w, "QUEUED")
			continue
		}
		switch upper {
		case "PING":
			writeSimple(w, "PONG")
		case "MULTI":
			tx = [][]string{{"MULTI"}}
			writeSimple(w, "OK")
		case "EXEC":
			results := make([]respValue, 0, len(tx)-1)
			for _, queued := range tx[1:] {
				result, err := s.execCommand(queued)
				if err != nil {
					writeError(w, err.Error())
					return
				}
				results = append(results, result)
			}
			tx = nil
			writeRESPArray(w, results)
		default:
			result, err := s.execCommand(cmd)
			if err != nil {
				writeError(w, err.Error())
				return
			}
			writeRaw(w, result.raw())
		}
	}
}

type respValue interface{ raw() string }

type respInteger int64

type respBulk string
type respNilBulk struct{}

func (v respInteger) raw() string { return fmt.Sprintf(":%d\r\n", v) }
func (v respBulk) raw() string    { return fmt.Sprintf("$%d\r\n%s\r\n", len(string(v)), string(v)) }
func (respNilBulk) raw() string   { return "$-1\r\n" }

func (s *fakeRedisServer) execCommand(cmd []string) (respValue, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	switch strings.ToUpper(cmd[0]) {
	case "RPUSH":
		key, value := cmd[1], cmd[2]
		s.lists[key] = append(s.lists[key], value)
		return respInteger(len(s.lists[key])), nil
	case "LPUSH":
		key, value := cmd[1], cmd[2]
		s.lists[key] = append([]string{value}, s.lists[key]...)
		return respInteger(len(s.lists[key])), nil
	case "BRPOPLPUSH":
		source, dest := cmd[1], cmd[2]
		items := s.lists[source]
		if len(items) == 0 {
			return respNilBulk{}, nil
		}
		value := items[len(items)-1]
		s.lists[source] = items[:len(items)-1]
		s.lists[dest] = append([]string{value}, s.lists[dest]...)
		return respBulk(value), nil
	case "LREM":
		key := cmd[1]
		count, _ := strconv.Atoi(cmd[2])
		value := cmd[3]
		items := s.lists[key]
		removed := 0
		out := make([]string, 0, len(items))
		for _, item := range items {
			if item == value && removed < count {
				removed++
				continue
			}
			out = append(out, item)
		}
		s.lists[key] = out
		return respInteger(removed), nil
	default:
		return nil, fmt.Errorf("unsupported command %q", cmd[0])
	}
}

func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	if line[0] != '*' {
		return nil, fmt.Errorf("unexpected prefix %q", line)
	}
	count, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
	parts := make([]string, 0, count)
	for i := 0; i < count; i++ {
		szLine, err := r.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sz, _ := strconv.Atoi(strings.TrimSpace(szLine[1:]))
		buf := make([]byte, sz+2)
		if _, err := r.Read(buf); err != nil {
			return nil, err
		}
		parts = append(parts, string(buf[:sz]))
	}
	return parts, nil
}

func writeSimple(w *bufio.Writer, msg string) { writeRaw(w, "+"+msg+"\r\n") }
func writeError(w *bufio.Writer, msg string)  { writeRaw(w, "-ERR "+msg+"\r\n") }
func writeRESPArray(w *bufio.Writer, values []respValue) {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("*%d\r\n", len(values)))
	for _, v := range values {
		b.WriteString(v.raw())
	}
	writeRaw(w, b.String())
}
func writeRaw(w *bufio.Writer, raw string) {
	_, _ = w.WriteString(raw)
	_ = w.Flush()
}

func TestReserveAckAndRequeue(t *testing.T) {
	t.Parallel()
	server := newFakeRedisServer(t)
	defer server.close()

	q := NewRedisQueue(server.addr(), "", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	job := DispatchJob{JobID: "job-1", NotificationID: "notif-1", AttemptID: "attempt-1", Channel: "email", CreatedAt: time.Now().UTC()}
	if err := q.EnqueueChannel(ctx, job); err != nil {
		t.Fatalf("EnqueueChannel() error = %v", err)
	}
	reserved, err := q.ReserveChannel(ctx, DispatchEmailQueueName, 1)
	if err != nil {
		t.Fatalf("ReserveChannel() error = %v", err)
	}
	if reserved.Job.JobID != job.JobID {
		t.Fatalf("reserved job = %+v", reserved.Job)
	}
	if err := q.RequeueReserved(ctx, reserved); err != nil {
		t.Fatalf("RequeueReserved() error = %v", err)
	}

	server.mu.Lock()
	if got := len(server.lists[DispatchEmailQueueName]); got != 1 {
		server.mu.Unlock()
		t.Fatalf("queue length after requeue = %d, want 1", got)
	}
	if got := len(server.lists[ProcessingQueueName(DispatchEmailQueueName)]); got != 0 {
		server.mu.Unlock()
		t.Fatalf("processing queue length after requeue = %d, want 0", got)
	}
	server.mu.Unlock()

	reserved, err = q.ReserveChannel(ctx, DispatchEmailQueueName, 1)
	if err != nil {
		t.Fatalf("ReserveChannel() second error = %v", err)
	}
	if err := q.AckReserved(ctx, reserved); err != nil {
		t.Fatalf("AckReserved() error = %v", err)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if got := len(server.lists[DispatchEmailQueueName]); got != 0 {
		t.Fatalf("queue length final = %d, want 0", got)
	}
	if got := len(server.lists[ProcessingQueueName(DispatchEmailQueueName)]); got != 0 {
		t.Fatalf("processing queue length final = %d, want 0", got)
	}
}

func TestRequeueReservedParsesRealExecIntegerReplies(t *testing.T) {
	t.Parallel()
	server := newFakeRedisServer(t)
	defer server.close()

	q := NewRedisQueue(server.addr(), "", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	job := DispatchJob{JobID: "job-2", NotificationID: "notif-2", AttemptID: "attempt-2", Channel: "webhook", CreatedAt: time.Now().UTC()}
	if err := q.EnqueueChannel(ctx, job); err != nil {
		t.Fatalf("EnqueueChannel() error = %v", err)
	}
	reserved, err := q.ReserveChannel(ctx, DispatchWebhookQueueName, 1)
	if err != nil {
		t.Fatalf("ReserveChannel() error = %v", err)
	}
	if err := q.RequeueReserved(ctx, reserved); err != nil {
		t.Fatalf("RequeueReserved() error = %v", err)
	}
}

func TestReserveChannelReturnsUnmarshalErrorWithoutAcknowledging(t *testing.T) {
	t.Parallel()
	server := newFakeRedisServer(t)
	defer server.close()
	payload := []byte("not-json")
	server.mu.Lock()
	server.lists[DispatchWebhookQueueName] = []string{string(payload)}
	server.mu.Unlock()

	q := NewRedisQueue(server.addr(), "", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, err := q.ReserveChannel(ctx, DispatchWebhookQueueName, 1)
	if err == nil {
		t.Fatal("ReserveChannel() error = nil, want unmarshal error")
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if got := len(server.lists[ProcessingQueueName(DispatchWebhookQueueName)]); got != 1 {
		t.Fatalf("processing queue length = %d, want 1", got)
	}
}
