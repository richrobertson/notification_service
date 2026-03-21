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
	ln       net.Listener
	mu       sync.Mutex
	lists    map[string][]string
	counts   map[string]int64
	expires  map[string]time.Time
	failNext map[string]error
}

func newFakeRedisServer(t *testing.T) *fakeRedisServer {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeRedisServer{ln: ln, lists: map[string][]string{}, counts: map[string]int64{}, expires: map[string]time.Time{}, failNext: map[string]error{}}
	go s.serve(t)
	return s
}

func (s *fakeRedisServer) addr() string { return s.ln.Addr().String() }
func (s *fakeRedisServer) close()       { _ = s.ln.Close() }
func (s *fakeRedisServer) failOnce(key string, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failNext[key] = err
}

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
			if err := s.consumeFailureLocked("MULTI"); err != nil {
				writeError(w, err.Error())
				continue
			}
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

func (s *fakeRedisServer) consumeFailureLocked(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err, ok := s.failNext[key]; ok {
		delete(s.failNext, key)
		return err
	}
	return nil
}

type respValue interface{ raw() string }

type respInteger int64

type respBulk string
type respNilBulk struct{}

func (v respInteger) raw() string { return fmt.Sprintf(":%d\r\n", v) }
func (v respBulk) raw() string    { return fmt.Sprintf("$%d\r\n%s\r\n", len(string(v)), string(v)) }
func (respNilBulk) raw() string   { return "$-1\r\n" }

func (s *fakeRedisServer) execCommand(cmd []string) (respValue, error) {
	if err := s.consumeFailureLocked(strings.ToUpper(cmd[0]) + " " + strings.Join(cmd[1:], " ")); err != nil {
		return nil, err
	}
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
	case "BRPOPLPUSH", "RPOPLPUSH":
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
	case "LLEN":
		return respInteger(len(s.lists[cmd[1]])), nil
	case "INCR":
		key := cmd[1]
		s.counts[key]++
		return respInteger(s.counts[key]), nil
	case "EXPIRE":
		seconds, _ := strconv.Atoi(cmd[2])
		s.expires[cmd[1]] = time.Now().Add(time.Duration(seconds) * time.Second)
		return respInteger(1), nil
	case "TTL":
		exp, ok := s.expires[cmd[1]]
		if !ok {
			return respInteger(-1), nil
		}
		return respInteger(int(time.Until(exp).Seconds())), nil
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

func TestDispatchJobNotLostWhenChannelEnqueueFails(t *testing.T) {
	t.Parallel()
	server := newFakeRedisServer(t)
	defer server.close()

	q := NewRedisQueue(server.addr(), "", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	job := DispatchJob{JobID: "job-3", NotificationID: "notif-3", AttemptID: "attempt-3", Channel: "email", CreatedAt: time.Now().UTC()}
	if err := q.EnqueueDispatch(ctx, job); err != nil {
		t.Fatalf("EnqueueDispatch() error = %v", err)
	}
	reserved, err := q.ReserveDispatch(ctx, 1)
	if err != nil {
		t.Fatalf("ReserveDispatch() error = %v", err)
	}
	server.failOnce("RPUSH "+DispatchEmailQueueName+" "+reserved.payload, fmt.Errorf("enqueue failed"))
	if err := q.EnqueueChannel(ctx, reserved.Job); err == nil {
		t.Fatal("EnqueueChannel() error = nil, want failure")
	}
	if err := q.RequeueReserved(ctx, reserved); err != nil {
		t.Fatalf("RequeueReserved() error = %v", err)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if got := len(server.lists[DispatchQueueName]); got != 1 {
		t.Fatalf("dispatch queue length = %d, want 1", got)
	}
	if got := len(server.lists[ProcessingQueueName(DispatchQueueName)]); got != 0 {
		t.Fatalf("dispatch processing queue length = %d, want 0", got)
	}
}

func TestDispatchJobAckRemovesSourceReservationAfterSuccessfulRoute(t *testing.T) {
	t.Parallel()
	server := newFakeRedisServer(t)
	defer server.close()

	q := NewRedisQueue(server.addr(), "", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	job := DispatchJob{JobID: "job-4", NotificationID: "notif-4", AttemptID: "attempt-4", Channel: "webhook", CreatedAt: time.Now().UTC()}
	if err := q.EnqueueDispatch(ctx, job); err != nil {
		t.Fatalf("EnqueueDispatch() error = %v", err)
	}
	reserved, err := q.ReserveDispatch(ctx, 1)
	if err != nil {
		t.Fatalf("ReserveDispatch() error = %v", err)
	}
	if err := q.EnqueueChannel(ctx, reserved.Job); err != nil {
		t.Fatalf("EnqueueChannel() error = %v", err)
	}
	if err := q.AckReserved(ctx, reserved); err != nil {
		t.Fatalf("AckReserved() error = %v", err)
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if got := len(server.lists[ProcessingQueueName(DispatchQueueName)]); got != 0 {
		t.Fatalf("dispatch processing queue length = %d, want 0", got)
	}
	if got := len(server.lists[DispatchWebhookQueueName]); got != 1 {
		t.Fatalf("target queue length = %d, want 1", got)
	}
}

func TestDispatchJobRemainsReservedWhenRequeueFails(t *testing.T) {
	t.Parallel()
	server := newFakeRedisServer(t)
	defer server.close()

	q := NewRedisQueue(server.addr(), "", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	job := DispatchJob{JobID: "job-5", NotificationID: "notif-5", AttemptID: "attempt-5", Channel: "email", CreatedAt: time.Now().UTC()}
	if err := q.EnqueueDispatch(ctx, job); err != nil {
		t.Fatalf("EnqueueDispatch() error = %v", err)
	}
	reserved, err := q.ReserveDispatch(ctx, 1)
	if err != nil {
		t.Fatalf("ReserveDispatch() error = %v", err)
	}
	server.failOnce("MULTI", fmt.Errorf("multi failed"))
	if err := q.RequeueReserved(ctx, reserved); err == nil {
		t.Fatal("RequeueReserved() error = nil, want failure")
	}

	server.mu.Lock()
	defer server.mu.Unlock()
	if got := len(server.lists[DispatchQueueName]); got != 0 {
		t.Fatalf("dispatch queue length = %d, want 0", got)
	}
	if got := len(server.lists[ProcessingQueueName(DispatchQueueName)]); got != 1 {
		t.Fatalf("dispatch processing queue length = %d, want 1", got)
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

func TestRecoverProcessingQueueMovesReservedJobsBack(t *testing.T) {
	t.Parallel()
	server := newFakeRedisServer(t)
	defer server.close()
	q := NewRedisQueue(server.addr(), "", 0)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	job := DispatchJob{JobID: "job-1", NotificationID: "notif-1", AttemptID: "attempt-1", Channel: "email", CreatedAt: time.Now().UTC()}
	if err := q.EnqueueChannel(ctx, job); err != nil {
		t.Fatal(err)
	}
	reserved, err := q.ReserveChannel(ctx, DispatchEmailQueueName, 1)
	if err != nil {
		t.Fatal(err)
	}
	if reserved.Job.JobID != job.JobID {
		t.Fatalf("reserved=%+v", reserved.Job)
	}
	recovered, err := q.RecoverProcessingQueue(ctx, DispatchEmailQueueName)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("recovered=%d", recovered)
	}
	server.mu.Lock()
	defer server.mu.Unlock()
	if got := len(server.lists[DispatchEmailQueueName]); got != 1 {
		t.Fatalf("queue len=%d", got)
	}
	if got := len(server.lists[ProcessingQueueName(DispatchEmailQueueName)]); got != 0 {
		t.Fatalf("processing len=%d", got)
	}
}
