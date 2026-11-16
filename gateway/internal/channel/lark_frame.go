// Copyright (c) 2026 MQ Global — GOSO Gateway. Clean-room implementation.

package channel

import (
	"fmt"
	"strconv"

	"google.golang.org/protobuf/encoding/protowire"
)

// larkFrame is one message on Lark's long connection: a small protobuf
// (fields 1 SeqID, 2 LogID, 3 service, 4 method, 5 headers, 6 payload
// encoding, 7 payload type, 8 payload, 9 LogIDNew). Method 0 is control
// (ping/pong), 1 is data (events).
type larkFrame struct {
	SeqID           uint64
	LogID           uint64
	Service         int32
	Method          int32
	Headers         [][2]string
	PayloadEncoding string
	PayloadType     string
	Payload         []byte
	LogIDNew        string
}

func (f larkFrame) encode() []byte {
	var b []byte
	b = protowire.AppendTag(b, 1, protowire.VarintType)
	b = protowire.AppendVarint(b, f.SeqID)
	b = protowire.AppendTag(b, 2, protowire.VarintType)
	b = protowire.AppendVarint(b, f.LogID)
	b = protowire.AppendTag(b, 3, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(int64(f.Service)))
	b = protowire.AppendTag(b, 4, protowire.VarintType)
	b = protowire.AppendVarint(b, uint64(int64(f.Method)))
	for _, h := range f.Headers {
		var hb []byte
		hb = protowire.AppendTag(hb, 1, protowire.BytesType)
		hb = protowire.AppendString(hb, h[0])
		hb = protowire.AppendTag(hb, 2, protowire.BytesType)
		hb = protowire.AppendString(hb, h[1])
		b = protowire.AppendTag(b, 5, protowire.BytesType)
		b = protowire.AppendBytes(b, hb)
	}
	if f.PayloadEncoding != "" {
		b = protowire.AppendTag(b, 6, protowire.BytesType)
		b = protowire.AppendString(b, f.PayloadEncoding)
	}
	if f.PayloadType != "" {
		b = protowire.AppendTag(b, 7, protowire.BytesType)
		b = protowire.AppendString(b, f.PayloadType)
	}
	if f.Payload != nil {
		b = protowire.AppendTag(b, 8, protowire.BytesType)
		b = protowire.AppendBytes(b, f.Payload)
	}
	if f.LogIDNew != "" {
		b = protowire.AppendTag(b, 9, protowire.BytesType)
		b = protowire.AppendString(b, f.LogIDNew)
	}
	return b
}

func decodeLarkFrame(b []byte) (larkFrame, error) {
	var f larkFrame
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return f, fmt.Errorf("lark frame: bad tag")
		}
		b = b[n:]
		switch typ {
		case protowire.VarintType:
			v, n := protowire.ConsumeVarint(b)
			if n < 0 {
				return f, fmt.Errorf("lark frame: bad varint")
			}
			b = b[n:]
			switch num {
			case 1:
				f.SeqID = v
			case 2:
				f.LogID = v
			case 3:
				f.Service = int32(v)
			case 4:
				f.Method = int32(v)
			}
		case protowire.BytesType:
			v, n := protowire.ConsumeBytes(b)
			if n < 0 {
				return f, fmt.Errorf("lark frame: bad bytes")
			}
			b = b[n:]
			switch num {
			case 5:
				f.Headers = append(f.Headers, decodeLarkHeader(v))
			case 6:
				f.PayloadEncoding = string(v)
			case 7:
				f.PayloadType = string(v)
			case 8:
				f.Payload = append([]byte(nil), v...)
			case 9:
				f.LogIDNew = string(v)
			}
		default:
			n := protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return f, fmt.Errorf("lark frame: bad field")
			}
			b = b[n:]
		}
	}
	return f, nil
}

func decodeLarkHeader(b []byte) [2]string {
	var h [2]string
	for len(b) > 0 {
		num, typ, n := protowire.ConsumeTag(b)
		if n < 0 {
			return h
		}
		b = b[n:]
		if typ != protowire.BytesType {
			n = protowire.ConsumeFieldValue(num, typ, b)
			if n < 0 {
				return h
			}
			b = b[n:]
			continue
		}
		v, n := protowire.ConsumeBytes(b)
		if n < 0 {
			return h
		}
		b = b[n:]
		if num == 1 {
			h[0] = string(v)
		} else if num == 2 {
			h[1] = string(v)
		}
	}
	return h
}

func (f larkFrame) header(key string) string {
	for _, h := range f.Headers {
		if h[0] == key {
			return h[1]
		}
	}
	return ""
}

// assemble joins an event Lark split across frames ("sum" parts, "seq" index,
// one "message_id"). whole is false until the last part arrives.
func (f larkFrame) assemble(parts map[string][][]byte) (payload []byte, whole bool) {
	sum, _ := strconv.Atoi(f.header("sum"))
	if sum <= 1 {
		return f.Payload, true
	}
	seq, _ := strconv.Atoi(f.header("seq"))
	id := f.header("message_id")
	if seq < 0 || seq >= sum || sum > 64 {
		return nil, false
	}
	got := parts[id]
	if len(got) != sum {
		got = make([][]byte, sum)
	}
	got[seq] = f.Payload
	for _, p := range got {
		if p == nil {
			parts[id] = got
			return nil, false
		}
	}
	delete(parts, id)
	var out []byte
	for _, p := range got {
		out = append(out, p...)
	}
	return out, true
}
