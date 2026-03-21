package worker

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/richrobertson/notification-platform/internal/delivery"
	"github.com/richrobertson/notification-platform/internal/queue"
)

type fakeRedisServer struct {
	ln    net.Listener
	mu    sync.Mutex
	lists map[string][]string
}

func newFakeRedisServerForWorker(t *testing.T) *fakeRedisServer {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	s := &fakeRedisServer{ln: ln, lists: map[string][]string{}}
	go s.serve()
	return s
}
func (s *fakeRedisServer) addr() string { return s.ln.Addr().String() }
func (s *fakeRedisServer) close()       { _ = s.ln.Close() }
func (s *fakeRedisServer) serve() {
	for {
		c, err := s.ln.Accept()
		if err != nil {
			return
		}
		go s.handle(c)
	}
}
func (s *fakeRedisServer) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	w := bufio.NewWriter(conn)
	for {
		cmd, err := readCommand(r)
		if err != nil {
			return
		}
		switch strings.ToUpper(cmd[0]) {
		case "PING":
			writeRaw(w, "+PONG\r\n")
		case "RPUSH":
			s.mu.Lock()
			s.lists[cmd[1]] = append(s.lists[cmd[1]], cmd[2])
			n := len(s.lists[cmd[1]])
			s.mu.Unlock()
			writeRaw(w, fmt.Sprintf(":%d\r\n", n))
		case "BRPOPLPUSH":
			s.mu.Lock()
			items := s.lists[cmd[1]]
			if len(items) == 0 {
				s.mu.Unlock()
				writeRaw(w, "$-1\r\n")
				continue
			}
			v := items[len(items)-1]
			s.lists[cmd[1]] = items[:len(items)-1]
			s.lists[cmd[2]] = append([]string{v}, s.lists[cmd[2]]...)
			s.mu.Unlock()
			writeRaw(w, fmt.Sprintf("$%d\r\n%s\r\n", len(v), v))
		case "LREM":
			s.mu.Lock()
			items := s.lists[cmd[1]]
			out := items[:0]
			removed := 0
			for _, item := range items {
				if item == cmd[3] && removed == 0 {
					removed++
					continue
				}
				out = append(out, item)
			}
			s.lists[cmd[1]] = out
			s.mu.Unlock()
			writeRaw(w, fmt.Sprintf(":%d\r\n", removed))
		default:
			writeRaw(w, "-ERR unsupported\r\n")
			return
		}
	}
}
func readCommand(r *bufio.Reader) ([]string, error) {
	line, err := r.ReadString('\n')
	if err != nil {
		return nil, err
	}
	count, _ := strconv.Atoi(strings.TrimSpace(line[1:]))
	parts := make([]string, 0, count)
	for i := 0; i < count; i++ {
		szLine, _ := r.ReadString('\n')
		sz, _ := strconv.Atoi(strings.TrimSpace(szLine[1:]))
		buf := make([]byte, sz+2)
		if _, err := r.Read(buf); err != nil {
			return nil, err
		}
		parts = append(parts, string(buf[:sz]))
	}
	return parts, nil
}
func writeRaw(w *bufio.Writer, raw string) { _, _ = w.WriteString(raw); _ = w.Flush() }

func TestFairSchedulerRotatesTenants(t *testing.T) {
	s := newFairScheduler(1, 1)
	s.add(queue.ReservedJob{Job: queue.DispatchJob{JobID: "a1", TenantID: "tenant-a"}})
	s.add(queue.ReservedJob{Job: queue.DispatchJob{JobID: "a2", TenantID: "tenant-a"}})
	s.add(queue.ReservedJob{Job: queue.DispatchJob{JobID: "b1", TenantID: "tenant-b"}})
	j1, _ := s.nextJob()
	s.complete(j1.Job.TenantID)
	j2, _ := s.nextJob()
	s.complete(j2.Job.TenantID)
	if j1.Job.TenantID == j2.Job.TenantID {
		t.Fatalf("scheduler did not rotate tenants: %s then %s", j1.Job.TenantID, j2.Job.TenantID)
	}
}

func TestRunChannelWorkerRespectsConcurrency(t *testing.T) {
	server := newFakeRedisServerForWorker(t)
	defer server.close()
	q := queue.NewRedisQueue(server.addr(), "", 0)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	for i := 0; i < 4; i++ {
		_ = q.EnqueueChannel(ctx, queue.DispatchJob{JobID: fmt.Sprintf("job-%d", i), AttemptID: fmt.Sprintf("a-%d", i), TenantID: fmt.Sprintf("t-%d", i%2), Channel: "email", CreatedAt: time.Now()})
	}
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	var mu sync.Mutex
	active, maxActive, processed := 0, 0, 0
	go RunChannelWorker(ctx, logger, q, queue.DispatchEmailQueueName, time.Second, func(ctx context.Context, job queue.DispatchJob) (delivery.Result, error) {
		mu.Lock()
		active++
		if active > maxActive {
			maxActive = active
		}
		mu.Unlock()
		time.Sleep(30 * time.Millisecond)
		mu.Lock()
		active--
		processed++
		done := processed == 4
		mu.Unlock()
		if done {
			cancel()
		}
		return delivery.Result{Outcome: delivery.OutcomeSent}, nil
	}, Options{Concurrency: 2, TenantBurst: 1, TenantMaxInFlight: 1})
	<-ctx.Done()
	time.Sleep(20 * time.Millisecond)
	if maxActive > 2 {
		t.Fatalf("max concurrency=%d", maxActive)
	}
}
