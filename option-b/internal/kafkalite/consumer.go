package kafkalite

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
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

// LatestOffset returns the next offset after the newest currently committed
// message. Tail-style pollers use it to ignore historical messages.
func (c *Consumer) LatestOffset(topic string, partition int32) (int64, error) {
	return c.lookupOffset(topic, partition, -1)
}

// EarliestOffset returns the first available offset for a topic partition.
func (c *Consumer) EarliestOffset(topic string, partition int32) (int64, error) {
	return c.lookupOffset(topic, partition, -2)
}

// FetchAll reads available messages from a single topic partition.
func (c *Consumer) FetchAll(topic string, partition int32, offset int64) ([]Message, error) {
	// Read from each configured broker until one responds. The caller passes the
	// next offset, which lets session replay poll incrementally.
	if c == nil || len(c.brokers) == 0 {
		return nil, errors.New("kafka broker is not configured")
	}

	if leader, err := c.leaderBroker(topic, partition); err == nil && leader != "" {
		if messages, err := c.fetchFromBroker(leader, topic, partition, offset); err == nil {
			return messages, nil
		}
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

func (c *Consumer) lookupOffset(topic string, partition int32, timestamp int64) (int64, error) {
	if c == nil || len(c.brokers) == 0 {
		return 0, errors.New("kafka broker is not configured")
	}

	if leader, err := c.leaderBroker(topic, partition); err == nil && leader != "" {
		if offset, err := c.lookupOffsetFromBroker(leader, topic, partition, timestamp); err == nil {
			return offset, nil
		}
	}

	var lastErr error
	for _, broker := range c.brokers {
		offset, err := c.lookupOffsetFromBroker(broker, topic, partition, timestamp)
		if err != nil {
			lastErr = err
			continue
		}
		return offset, nil
	}
	if lastErr != nil {
		return 0, lastErr
	}
	return 0, errors.New("kafka offset lookup failed")
}

func (c *Consumer) leaderBroker(topic string, partition int32) (string, error) {
	for _, broker := range c.brokers {
		leader, err := c.fetchLeaderFromBroker(broker, topic, partition)
		if err != nil {
			continue
		}
		if leader != "" {
			return leader, nil
		}
	}
	return "", errors.New("kafka metadata leader not found")
}

func (c *Consumer) fetchLeaderFromBroker(broker, topic string, partition int32) (string, error) {
	conn, err := net.DialTimeout("tcp", broker, 5*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	var req bytes.Buffer
	writeInt16(&req, 3) // Metadata API
	writeInt16(&req, 0) // Metadata v0
	writeInt32(&req, 3) // correlation id
	writeString(&req, "rotr-go-engine")
	writeInt32(&req, 1) // topic count
	writeString(&req, topic)

	var framed bytes.Buffer
	writeInt32(&framed, int32(req.Len()))
	framed.Write(req.Bytes())
	if _, err := conn.Write(framed.Bytes()); err != nil {
		return "", err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return "", err
	}
	size := int(binary.BigEndian.Uint32(header))
	if size <= 0 {
		return "", errors.New("empty metadata response")
	}
	resp := make([]byte, size)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return "", err
	}
	return parseLeaderMetadata(resp, topic, partition)
}

func (c *Consumer) lookupOffsetFromBroker(broker, topic string, partition int32, timestamp int64) (int64, error) {
	conn, err := net.DialTimeout("tcp", broker, 5*time.Second)
	if err != nil {
		return 0, err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	var req bytes.Buffer
	writeInt16(&req, 2) // ListOffsets API
	writeInt16(&req, 0) // ListOffsets v0
	writeInt32(&req, 4) // correlation id
	writeString(&req, "rotr-go-engine")
	writeInt32(&req, -1) // replica id: ordinary consumer
	writeInt32(&req, 1)  // topic count
	writeString(&req, topic)
	writeInt32(&req, 1) // partition count
	writeInt32(&req, partition)
	writeInt64(&req, timestamp)
	writeInt32(&req, 1) // max offsets

	var framed bytes.Buffer
	writeInt32(&framed, int32(req.Len()))
	framed.Write(req.Bytes())
	if _, err := conn.Write(framed.Bytes()); err != nil {
		return 0, err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return 0, err
	}
	size := int(binary.BigEndian.Uint32(header))
	if size <= 0 {
		return 0, errors.New("empty offset response")
	}
	resp := make([]byte, size)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return 0, err
	}
	return parseOffsetResponse(resp)
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

func parseLeaderMetadata(resp []byte, wantedTopic string, wantedPartition int32) (string, error) {
	pos := 4 // correlation id
	brokerCount, next, ok := readInt32(resp, pos)
	if !ok {
		return "", errors.New("short metadata broker count")
	}
	pos = next

	brokers := make(map[int32]string, brokerCount)
	for i := int32(0); i < brokerCount; i++ {
		nodeID, next, ok := readInt32(resp, pos)
		if !ok {
			return "", errors.New("short metadata broker id")
		}
		host, next, ok := readStringValue(resp, next)
		if !ok {
			return "", errors.New("short metadata broker host")
		}
		port, next, ok := readInt32(resp, next)
		if !ok {
			return "", errors.New("short metadata broker port")
		}
		brokers[nodeID] = fmt.Sprintf("%s:%d", host, port)
		pos = next
	}

	topicCount, next, ok := readInt32(resp, pos)
	if !ok {
		return "", errors.New("short metadata topic count")
	}
	pos = next
	for i := int32(0); i < topicCount; i++ {
		topicError, next, ok := readInt16(resp, pos)
		if !ok {
			return "", errors.New("short metadata topic error")
		}
		topicName, next, ok := readStringValue(resp, next)
		if !ok {
			return "", errors.New("short metadata topic name")
		}
		partitionCount, next, ok := readInt32(resp, next)
		if !ok {
			return "", errors.New("short metadata partition count")
		}
		pos = next
		for p := int32(0); p < partitionCount; p++ {
			partitionError, next, ok := readInt16(resp, pos)
			if !ok {
				return "", errors.New("short metadata partition error")
			}
			partitionID, next, ok := readInt32(resp, next)
			if !ok {
				return "", errors.New("short metadata partition id")
			}
			leaderID, next, ok := readInt32(resp, next)
			if !ok {
				return "", errors.New("short metadata leader id")
			}
			replicaCount, next, ok := readInt32(resp, next)
			if !ok {
				return "", errors.New("short metadata replica count")
			}
			next += int(replicaCount) * 4
			isrCount, nextAfterReplicas, ok := readInt32(resp, next)
			if !ok {
				return "", errors.New("short metadata isr count")
			}
			next = nextAfterReplicas + int(isrCount)*4
			if next > len(resp) {
				return "", errors.New("short metadata replica/isr data")
			}
			pos = next

			if topicError == 0 && partitionError == 0 && topicName == wantedTopic && partitionID == wantedPartition {
				leader := brokers[leaderID]
				if leader == "" {
					return "", errors.New("metadata leader broker missing")
				}
				return leader, nil
			}
		}
	}
	return "", errors.New("metadata partition not found")
}

func parseOffsetResponse(resp []byte) (int64, error) {
	pos := 4 // correlation id
	topicCount, next, ok := readInt32(resp, pos)
	if !ok {
		return 0, errors.New("short offset topic count")
	}
	pos = next
	if topicCount < 1 {
		return 0, errors.New("offset response has no topics")
	}
	_, next, ok = readStringValue(resp, pos)
	if !ok {
		return 0, errors.New("short offset topic name")
	}
	partitionCount, next, ok := readInt32(resp, next)
	if !ok {
		return 0, errors.New("short offset partition count")
	}
	pos = next
	if partitionCount < 1 {
		return 0, errors.New("offset response has no partitions")
	}
	_, next, ok = readInt32(resp, pos) // partition
	if !ok {
		return 0, errors.New("short offset partition id")
	}
	errorCode, next, ok := readInt16(resp, next)
	if !ok {
		return 0, errors.New("short offset error code")
	}
	if errorCode != 0 {
		return 0, errors.New("kafka offset error code " + strconv.Itoa(int(errorCode)))
	}
	offsetCount, next, ok := readInt32(resp, next)
	if !ok {
		return 0, errors.New("short offset count")
	}
	if offsetCount < 1 || next+8 > len(resp) {
		return 0, errors.New("offset response has no offsets")
	}
	return int64(binary.BigEndian.Uint64(resp[next : next+8])), nil
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
		return nil, errors.New("kafka fetch error code " + strconv.Itoa(int(errorCode)))
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

func readInt16(data []byte, pos int) (int16, int, bool) {
	if pos+2 > len(data) {
		return 0, pos, false
	}
	return int16(binary.BigEndian.Uint16(data[pos : pos+2])), pos + 2, true
}

func readInt32(data []byte, pos int) (int32, int, bool) {
	if pos+4 > len(data) {
		return 0, pos, false
	}
	return int32(binary.BigEndian.Uint32(data[pos : pos+4])), pos + 4, true
}

func readStringValue(data []byte, pos int) (string, int, bool) {
	if pos+2 > len(data) {
		return "", pos, false
	}
	size := int(int16(binary.BigEndian.Uint16(data[pos : pos+2])))
	pos += 2
	if size < 0 || pos+size > len(data) {
		return "", pos, false
	}
	return string(data[pos : pos+size]), pos + size, true
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
