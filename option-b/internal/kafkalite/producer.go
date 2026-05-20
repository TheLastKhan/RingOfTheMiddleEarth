// Package kafkalite contains the small Kafka protocol surface this demo needs.
package kafkalite

import (
	"bytes"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

// Producer writes plain JSON messages to a Kafka topic without external deps.
type Producer struct {
	brokers []string
}

// NewProducer creates a producer using the first broker in a comma list.
func NewProducer(brokers string) *Producer {
	brokerList := []string{"kafka-1:29092"}
	if brokers != "" {
		brokerList = nil
		for _, broker := range strings.Split(brokers, ",") {
			broker = strings.TrimSpace(broker)
			if broker != "" {
				brokerList = append(brokerList, broker)
			}
		}
	}
	if len(brokerList) == 0 {
		brokerList = []string{"kafka-1:29092"}
	}
	return &Producer{brokers: brokerList}
}

// Produce sends one message to partition 0 with acks=1.
func (p *Producer) Produce(topic, key string, value []byte) error {
	if p == nil || len(p.brokers) == 0 {
		return errors.New("kafka broker is not configured")
	}

	var lastErr error
	for _, broker := range p.brokers {
		if err := p.produceToBroker(broker, topic, key, value); err != nil {
			lastErr = err
			continue
		}
		return nil
	}
	if lastErr != nil {
		return lastErr
	}
	return errors.New("kafka produce failed")
}

func (p *Producer) produceToBroker(broker, topic, key string, value []byte) error {
	conn, err := net.DialTimeout("tcp", broker, 5*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	_ = conn.SetDeadline(time.Now().Add(8 * time.Second))

	var req bytes.Buffer
	writeInt16(&req, 0) // Produce API
	writeInt16(&req, 0) // Produce v0
	writeInt32(&req, 1) // correlation id
	writeString(&req, "rotr-go-engine")

	writeInt16(&req, 1)    // acks
	writeInt32(&req, 5000) // timeout ms
	writeInt32(&req, 1)    // topic count
	writeString(&req, topic)
	partition := partitionFor(topic)
	writeInt32(&req, 1) // partition count
	writeInt32(&req, partition)

	messageSet := buildMessageSet(key, value)
	writeBytes(&req, messageSet)

	var framed bytes.Buffer
	writeInt32(&framed, int32(req.Len()))
	framed.Write(req.Bytes())

	if _, err := conn.Write(framed.Bytes()); err != nil {
		return err
	}

	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return err
	}
	size := int(binary.BigEndian.Uint32(header))
	if size <= 0 {
		return nil
	}
	resp := make([]byte, size)
	if _, err := io.ReadFull(conn, resp); err != nil {
		return err
	}
	if errorCode := firstProduceError(resp); errorCode != 0 {
		return errors.New("kafka produce error code " + strconv.Itoa(int(errorCode)))
	}
	return nil
}

func partitionFor(topic string) int32 {
	switch topic {
	case "game.orders.raw":
		return 2
	case "game.orders.validated", "game.events.path", "game.broadcast", "game.ring.position", "game.ring.detection":
		return 0
	case "game.session":
		return 0
	case "game.events.unit", "game.events.region":
		return 1
	default:
		return 0
	}
}

func firstProduceError(resp []byte) int16 {
	if len(resp) < 18 {
		return -1
	}
	pos := 4 // correlation id
	if pos+4 > len(resp) {
		return -1
	}
	topicCount := int(binary.BigEndian.Uint32(resp[pos : pos+4]))
	pos += 4
	if topicCount < 1 || pos+2 > len(resp) {
		return -1
	}
	topicLen := int(binary.BigEndian.Uint16(resp[pos : pos+2]))
	pos += 2 + topicLen
	if pos+4 > len(resp) {
		return -1
	}
	partitionCount := int(binary.BigEndian.Uint32(resp[pos : pos+4]))
	pos += 4
	if partitionCount < 1 || pos+6 > len(resp) {
		return -1
	}
	pos += 4 // partition
	return int16(binary.BigEndian.Uint16(resp[pos : pos+2]))
}

func buildMessageSet(key string, value []byte) []byte {
	var msg bytes.Buffer
	msg.WriteByte(0) // magic
	msg.WriteByte(0) // attributes
	writeNullableBytes(&msg, []byte(key))
	writeNullableBytes(&msg, value)

	payload := msg.Bytes()
	crc := crc32.ChecksumIEEE(payload)

	var message bytes.Buffer
	writeInt32(&message, int32(crc))
	message.Write(payload)

	var set bytes.Buffer
	writeInt64(&set, 0)
	writeInt32(&set, int32(message.Len()))
	set.Write(message.Bytes())
	return set.Bytes()
}

func writeInt16(buf *bytes.Buffer, value int16) {
	_ = binary.Write(buf, binary.BigEndian, value)
}

func writeInt32(buf *bytes.Buffer, value int32) {
	_ = binary.Write(buf, binary.BigEndian, value)
}

func writeInt64(buf *bytes.Buffer, value int64) {
	_ = binary.Write(buf, binary.BigEndian, value)
}

func writeString(buf *bytes.Buffer, value string) {
	writeInt16(buf, int16(len(value)))
	buf.WriteString(value)
}

func writeBytes(buf *bytes.Buffer, value []byte) {
	writeInt32(buf, int32(len(value)))
	buf.Write(value)
}

func writeNullableBytes(buf *bytes.Buffer, value []byte) {
	if value == nil {
		writeInt32(buf, -1)
		return
	}
	writeBytes(buf, value)
}
