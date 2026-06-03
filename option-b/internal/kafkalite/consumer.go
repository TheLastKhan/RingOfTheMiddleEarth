package kafkalite

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"time"
)

// Message is the tiny Kafka message shape needed for session replay.
type Message struct {
	Offset int64
	Key    []byte
	Value  []byte
}

// Consumer reads plain Kafka messages without external dependencies.
type Consumer struct {
	brokers []string
}

func NewConsumer(brokers string) *Consumer {
	// Reuse the producer's broker parsing so both Kafka helpers interpret
	// KAFKA_BROKERS the same way.
	return &Consumer{brokers: NewProducer(brokers).brokers}
}

// FetchAll reads available messages from a single topic partition.
func (c *Consumer) FetchAll(topic string, partition int32, offset int64) ([]Message, error) {
	// Read from each configured broker until one responds. The caller passes the
	// next offset, which lets session replay poll incrementally.
	if c == nil || len(c.brokers) == 0 {
		return nil, errors.New("kafka broker is not configured")
	}

	var lastErr error
	for _, broker := range c.brokers {
		messages, err := c.fetchFromBroker(broker, topic, partition, offset)
		if err != nil {
			lastErr = err
			continue
		}
		return messages, nil
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, errors.New("kafka fetch failed")
}

func (c *Consumer) fetchFromBroker(broker, topic string, partition int32, offset int64) ([]Message, error) {
	// Minimal Kafka Fetch v0 request. This is intentionally scoped to what the
	// engine needs: reading JSON snapshots from one topic partition.
	conn, err := net.DialTimeout("tcp", broker, 5*time.Second)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	var req bytes.Buffer
	writeInt16(&req, 1) // Fetch API
	writeInt16(&req, 0) // Fetch v0
	writeInt32(&req, 2) // correlation id
	writeString(&req, "rotr-go-engine")
	writeInt32(&req, -1)   // replica id: ordinary consumer
	writeInt32(&req, 1000) // max wait ms
	writeInt32(&req, 1)    // min bytes
	writeInt32(&req, 1)    // topic count
	writeString(&req, topic)
	writeInt32(&req, 1) // partition count
	writeInt32(&req, partition)
	writeInt64(&req, offset)
	writeInt32(&req, 16*1024*1024)

	var framed bytes.Buffer
	writeInt32(&framed, int32(req.Len()))
	framed.Write(req.Bytes())
	if _, err := conn.Write(framed.Bytes()); err != nil {
		return nil, err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, err
	}
	size := int(binary.BigEndian.Uint32(header))
	if size <= 0 {
		return nil, nil
	}
	resp := make([]byte, size)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return nil, err
	}
	return parseFetchResponse(resp)
}

func parseFetchResponse(resp []byte) ([]Message, error) {
	// Parse enough of the Fetch v0 response to extract offset, key, and value
	// from each message. Unknown or malformed trailing bytes stop parsing.
	pos := 4 // correlation id
	if pos+4 > len(resp) {
		return nil, errors.New("short fetch response")
	}
	topicCount := int(binary.BigEndian.Uint32(resp[pos : pos+4]))
	pos += 4
	if topicCount < 1 || pos+2 > len(resp) {
		return nil, nil
	}
	topicLen := int(binary.BigEndian.Uint16(resp[pos : pos+2]))
	pos += 2 + topicLen
	if pos+4 > len(resp) {
		return nil, errors.New("short fetch topic")
	}
	partitionCount := int(binary.BigEndian.Uint32(resp[pos : pos+4]))
	pos += 4
	if partitionCount < 1 || pos+18 > len(resp) {
		return nil, nil
	}
	pos += 4 // partition
	errorCode := int16(binary.BigEndian.Uint16(resp[pos : pos+2]))
	pos += 2
	if errorCode != 0 {
		return nil, errors.New("kafka fetch error")
	}
	pos += 8 // high watermark
	if pos+4 > len(resp) {
		return nil, errors.New("short message set size")
	}
	messageSetSize := int(binary.BigEndian.Uint32(resp[pos : pos+4]))
	pos += 4
	end := pos + messageSetSize
	if end > len(resp) {
		end = len(resp)
	}

	var messages []Message
	for pos+12 <= end {
		offset := int64(binary.BigEndian.Uint64(resp[pos : pos+8]))
		pos += 8
		msgSize := int(binary.BigEndian.Uint32(resp[pos : pos+4]))
		pos += 4
		if msgSize < 6 || pos+msgSize > end {
			break
		}
		msgEnd := pos + msgSize
		pos += 4 // crc
		pos++    // magic
		pos++    // attributes
		key, next, ok := readNullableBytes(resp, pos, msgEnd)
		if !ok {
			break
		}
		value, next, ok := readNullableBytes(resp, next, msgEnd)
		if !ok {
			break
		}
		messages = append(messages, Message{Offset: offset, Key: key, Value: value})
		pos = msgEnd
	}
	return messages, nil
}

func readNullableBytes(data []byte, pos, end int) ([]byte, int, bool) {
	if pos+4 > end {
		return nil, pos, false
	}
	size := int(int32(binary.BigEndian.Uint32(data[pos : pos+4])))
	pos += 4
	if size < 0 {
		return nil, pos, true
	}
	if pos+size > end {
		return nil, pos, false
	}
	value := append([]byte(nil), data[pos:pos+size]...)
	return value, pos + size, true
}
