package queue

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"sync"
)

type RedisQueue struct {
	addr     string
	password string
	db       int

	mu   sync.Mutex
	conn net.Conn
	rw   *bufio.ReadWriter
}

type ReservedJob struct {
	Job             DispatchJob
	queueName       string
	processingQueue string
	payload         string
}

func NewRedisQueue(addr, password string, db int) *RedisQueue {
	return &RedisQueue{addr: addr, password: password, db: db}
}

func (q *RedisQueue) Close() error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.conn == nil {
		return nil
	}
	err := q.conn.Close()
	q.conn = nil
	q.rw = nil
	if err != nil {
		return fmt.Errorf("close redis connection: %w", err)
	}
	return nil
}

func (q *RedisQueue) Ping(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureConnLocked(ctx); err != nil {
		return err
	}
	if err := q.writeCommandLocked("PING"); err != nil {
		return err
	}
	response, err := q.readResponseLocked()
	if err != nil {
		q.resetConnLocked()
		return fmt.Errorf("ping redis: %w", err)
	}
	if response != "PONG" {
		return fmt.Errorf("ping redis: unexpected response %q", response)
	}
	return nil
}

func (q *RedisQueue) EnqueueDispatch(ctx context.Context, job DispatchJob) error {
	return q.enqueue(ctx, DispatchQueueName, job)
}

func (q *RedisQueue) EnqueueChannel(ctx context.Context, job DispatchJob) error {
	queueName, err := QueueNameForChannel(job.Channel)
	if err != nil {
		return err
	}
	return q.enqueue(ctx, queueName, job)
}

func (q *RedisQueue) ConsumeDispatch(ctx context.Context) (DispatchJob, error) {
	return q.ConsumeChannel(ctx, DispatchQueueName, 1)
}

func (q *RedisQueue) ConsumeChannel(ctx context.Context, queueName string, timeoutSeconds int) (DispatchJob, error) {
	reserved, err := q.ReserveChannel(ctx, queueName, timeoutSeconds)
	if err != nil {
		return DispatchJob{}, err
	}
	if err := q.AckReserved(ctx, reserved); err != nil {
		return DispatchJob{}, err
	}
	return reserved.Job, nil
}

func (q *RedisQueue) ReserveChannel(ctx context.Context, queueName string, timeoutSeconds int) (ReservedJob, error) {
	if timeoutSeconds <= 0 {
		timeoutSeconds = 1
	}
	processingQueue := ProcessingQueueName(queueName)
	for {
		if err := ctx.Err(); err != nil {
			return ReservedJob{}, err
		}
		payload, err := q.brpoplpush(ctx, queueName, processingQueue, timeoutSeconds)
		if err != nil {
			if errors.Is(err, errRedisNil) {
				continue
			}
			return ReservedJob{}, err
		}
		var job DispatchJob
		if err := json.Unmarshal([]byte(payload), &job); err != nil {
			return ReservedJob{}, fmt.Errorf("reserve %s job: unmarshal job: %w", queueName, err)
		}
		return ReservedJob{Job: job, queueName: queueName, processingQueue: processingQueue, payload: payload}, nil
	}
}

func (q *RedisQueue) AckReserved(ctx context.Context, reserved ReservedJob) error {
	return q.lrem(ctx, reserved.processingQueue, 1, reserved.payload)
}

func (q *RedisQueue) RequeueReserved(ctx context.Context, reserved ReservedJob) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureConnLocked(ctx); err != nil {
		return err
	}
	if err := q.writeCommandLocked("MULTI"); err != nil {
		return err
	}
	if _, err := q.readResponseLocked(); err != nil {
		q.resetConnLocked()
		return fmt.Errorf("requeue reserved job: start transaction: %w", err)
	}
	if err := q.writeCommandLocked("LREM", reserved.processingQueue, "1", reserved.payload); err != nil {
		return err
	}
	if _, err := q.readResponseLocked(); err != nil {
		q.resetConnLocked()
		return fmt.Errorf("requeue reserved job: queue lrem: %w", err)
	}
	if err := q.writeCommandLocked("LPUSH", reserved.queueName, reserved.payload); err != nil {
		return err
	}
	if _, err := q.readResponseLocked(); err != nil {
		q.resetConnLocked()
		return fmt.Errorf("requeue reserved job: queue lpush: %w", err)
	}
	if err := q.writeCommandLocked("EXEC"); err != nil {
		return err
	}
	response, err := q.readResponseLocked()
	if err != nil {
		q.resetConnLocked()
		return fmt.Errorf("requeue reserved job: exec: %w", err)
	}
	values, ok := response.([]string)
	if !ok || len(values) != 2 {
		return fmt.Errorf("requeue reserved job: unexpected exec response %#v", response)
	}
	return nil
}

func ProcessingQueueName(queueName string) string {
	return queueName + ":processing"
}

func QueueNameForChannel(channel string) (string, error) {
	switch channel {
	case "webhook":
		return DispatchWebhookQueueName, nil
	case "email":
		return DispatchEmailQueueName, nil
	default:
		return "", fmt.Errorf("unsupported channel %q", channel)
	}
}

func (q *RedisQueue) enqueue(ctx context.Context, queueName string, job DispatchJob) error {
	payload, err := json.Marshal(job)
	if err != nil {
		return fmt.Errorf("marshal dispatch job: %w", err)
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureConnLocked(ctx); err != nil {
		return err
	}
	if err := q.writeCommandLocked("RPUSH", queueName, string(payload)); err != nil {
		return err
	}
	if _, err := q.readResponseLocked(); err != nil {
		q.resetConnLocked()
		return fmt.Errorf("enqueue job on %s: %w", queueName, err)
	}
	return nil
}

func (q *RedisQueue) brpoplpush(ctx context.Context, sourceQueue, destinationQueue string, timeoutSeconds int) (string, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureConnLocked(ctx); err != nil {
		return "", err
	}
	if err := q.writeCommandLocked("BRPOPLPUSH", sourceQueue, destinationQueue, strconv.Itoa(timeoutSeconds)); err != nil {
		return "", err
	}
	response, err := q.readResponseLocked()
	if err != nil {
		q.resetConnLocked()
		return "", fmt.Errorf("reserve dispatch job: %w", err)
	}
	if response == nil {
		return "", errRedisNil
	}
	payload, ok := response.(string)
	if !ok {
		return "", fmt.Errorf("reserve dispatch job: unexpected response %#v", response)
	}
	return payload, nil
}

func (q *RedisQueue) lrem(ctx context.Context, queueName string, count int, payload string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if err := q.ensureConnLocked(ctx); err != nil {
		return err
	}
	if err := q.writeCommandLocked("LREM", queueName, strconv.Itoa(count), payload); err != nil {
		return err
	}
	response, err := q.readResponseLocked()
	if err != nil {
		q.resetConnLocked()
		return fmt.Errorf("remove reserved job from %s: %w", queueName, err)
	}
	removed, ok := response.(int64)
	if !ok {
		return fmt.Errorf("remove reserved job from %s: unexpected response %#v", queueName, response)
	}
	if removed == 0 {
		return fmt.Errorf("remove reserved job from %s: job not found", queueName)
	}
	return nil
}

func (q *RedisQueue) ensureConnLocked(ctx context.Context) error {
	if q.conn != nil {
		return nil
	}
	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", q.addr)
	if err != nil {
		return fmt.Errorf("connect to redis: %w", err)
	}
	q.conn = conn
	q.rw = bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))
	if q.password != "" {
		if err := q.writeCommandLocked("AUTH", q.password); err != nil {
			q.resetConnLocked()
			return err
		}
		if _, err := q.readResponseLocked(); err != nil {
			q.resetConnLocked()
			return fmt.Errorf("authenticate redis: %w", err)
		}
	}
	if q.db != 0 {
		if err := q.writeCommandLocked("SELECT", strconv.Itoa(q.db)); err != nil {
			q.resetConnLocked()
			return err
		}
		if _, err := q.readResponseLocked(); err != nil {
			q.resetConnLocked()
			return fmt.Errorf("select redis db: %w", err)
		}
	}
	return nil
}

func (q *RedisQueue) resetConnLocked() {
	if q.conn != nil {
		_ = q.conn.Close()
	}
	q.conn = nil
	q.rw = nil
}

func (q *RedisQueue) writeCommandLocked(parts ...string) error {
	if q.rw == nil {
		return fmt.Errorf("redis connection is not initialized")
	}
	if _, err := q.rw.WriteString(fmt.Sprintf("*%d\r\n", len(parts))); err != nil {
		return fmt.Errorf("write redis command: %w", err)
	}
	for _, part := range parts {
		if _, err := q.rw.WriteString(fmt.Sprintf("$%d\r\n%s\r\n", len(part), part)); err != nil {
			return fmt.Errorf("write redis command: %w", err)
		}
	}
	if err := q.rw.Flush(); err != nil {
		return fmt.Errorf("flush redis command: %w", err)
	}
	return nil
}

var errRedisNil = errors.New("redis nil response")

func (q *RedisQueue) readResponseLocked() (any, error) {
	if q.rw == nil {
		return nil, fmt.Errorf("redis connection is not initialized")
	}
	prefix, err := q.rw.ReadByte()
	if err != nil {
		if errors.Is(err, io.EOF) {
			return nil, io.EOF
		}
		return nil, err
	}
	line, err := q.rw.ReadString('\n')
	if err != nil {
		return nil, err
	}
	line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
	switch prefix {
	case '+':
		return line, nil
	case '-':
		return nil, fmt.Errorf("redis error: %s", line)
	case ':':
		value, err := strconv.ParseInt(line, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("parse redis integer %q: %w", line, err)
		}
		return value, nil
	case '$':
		size, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("parse redis bulk length %q: %w", line, err)
		}
		if size < 0 {
			return nil, nil
		}
		buf := make([]byte, size+2)
		if _, err := io.ReadFull(q.rw, buf); err != nil {
			return nil, err
		}
		return string(buf[:size]), nil
	case '*':
		count, err := strconv.Atoi(line)
		if err != nil {
			return nil, fmt.Errorf("parse redis array length %q: %w", line, err)
		}
		if count < 0 {
			return nil, nil
		}
		values := make([]string, 0, count)
		for i := 0; i < count; i++ {
			item, err := q.readResponseLocked()
			if err != nil {
				return nil, err
			}
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("unexpected redis array item %#v", item)
			}
			values = append(values, text)
		}
		return values, nil
	default:
		return nil, fmt.Errorf("unsupported redis response prefix %q", prefix)
	}
}
