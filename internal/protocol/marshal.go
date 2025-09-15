package protocol

import (
	"bytes"
	"sync"

	"github.com/tinylib/msgp/msgp"
)

// Pool of buffers to avoid allocation and ensure thread safety
var bufferPool = sync.Pool{
	New: func() interface{} {
		return &bytes.Buffer{}
	},
}

// Marshal serializes a message to msgpack format
func Marshal(v interface{}) ([]byte, error) {
	// Get a buffer from the pool to ensure thread safety
	buf := bufferPool.Get().(*bytes.Buffer)
	buf.Reset()
	defer bufferPool.Put(buf)
	
	writer := msgp.NewWriter(buf)

	switch msg := v.(type) {
	case *Connect:
		if err := msg.EncodeMsg(writer); err != nil {
			return nil, err
		}
	case *HandStart:
		if err := msg.EncodeMsg(writer); err != nil {
			return nil, err
		}
	case *GameUpdate:
		if err := msg.EncodeMsg(writer); err != nil {
			return nil, err
		}
	case *Action:
		if err := msg.EncodeMsg(writer); err != nil {
			return nil, err
		}
	case *ActionRequest:
		if err := msg.EncodeMsg(writer); err != nil {
			return nil, err
		}
	case *StreetChange:
		if err := msg.EncodeMsg(writer); err != nil {
			return nil, err
		}
	case *HandResult:
		if err := msg.EncodeMsg(writer); err != nil {
			return nil, err
		}
	case *Error:
		if err := msg.EncodeMsg(writer); err != nil {
			return nil, err
		}
	default:
		return nil, ErrUnknownMessageType
	}

	if err := writer.Flush(); err != nil {
		return nil, err
	}

	// Create a copy to avoid aliasing the pooled buffer
	out := make([]byte, buf.Len())
	copy(out, buf.Bytes())
	return out, nil
}

// Unmarshal deserializes msgpack data into a message
func Unmarshal(data []byte, v interface{}) error {
	reader := msgp.NewReader(bytes.NewReader(data))

	switch msg := v.(type) {
	case *Connect:
		return msg.DecodeMsg(reader)
	case *HandStart:
		return msg.DecodeMsg(reader)
	case *GameUpdate:
		return msg.DecodeMsg(reader)
	case *Action:
		return msg.DecodeMsg(reader)
	case *ActionRequest:
		return msg.DecodeMsg(reader)
	case *StreetChange:
		return msg.DecodeMsg(reader)
	case *HandResult:
		return msg.DecodeMsg(reader)
	case *Error:
		return msg.DecodeMsg(reader)
	default:
		return ErrUnknownMessageType
	}
}
