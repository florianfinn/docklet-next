// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original bounded Minecraft Java server list status client.

package widget

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"strconv"
	"strings"
	"unicode/utf8"

	dto "github.com/florianfinn/docklet-next/types/widget"
)

const minecraftLimit = 64 << 10

func varInt(w io.Writer, n int) error {
	var b [5]byte
	i := 0
	for {
		b[i] = byte(n & 0x7f)
		n >>= 7
		if n == 0 {
			break
		}
		b[i] |= 0x80
		i++
	}
	_, err := w.Write(b[:i+1])
	return err
}
func readVarInt(r io.ByteReader) (int, error) {
	n := uint32(0)
	for i := 0; i < 5; i++ {
		b, err := r.ReadByte()
		if err != nil {
			return 0, err
		}
		if i == 4 && b > 7 {
			return 0, dto.Failure(dto.BadResponse)
		}
		n |= uint32(b&0x7f) << uint(i*7)
		if b&0x80 == 0 {
			return int(n), nil
		}
	}
	return 0, dto.Failure(dto.BadResponse)
}
func packet(w io.Writer, b []byte) error {
	if err := varInt(w, len(b)); err != nil {
		return err
	}
	_, err := w.Write(b)
	return err
}

func fetchMinecraft(ctx context.Context, address string) (*dto.MinecraftData, error) {
	host, port, _ := net.SplitHostPort(address)
	n, _ := strconv.Atoi(port)
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, transportProblem(ctx, err)
	}
	defer conn.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	var handshake bytes.Buffer
	_ = varInt(&handshake, 0)
	_ = varInt(&handshake, mathProtocolVersion)
	_ = varInt(&handshake, len(host))
	handshake.WriteString(host)
	_ = binary.Write(&handshake, binary.BigEndian, uint16(n))
	_ = varInt(&handshake, 1)
	if err = packet(conn, handshake.Bytes()); err == nil {
		err = packet(conn, []byte{0})
	}
	if err != nil {
		return nil, transportProblem(ctx, err)
	}
	r := bufio.NewReader(conn)
	length, err := readVarInt(r)
	if err != nil {
		return nil, minecraftProblem(ctx, err)
	}
	if length < 3 || length > minecraftLimit {
		return nil, dto.Failure(dto.BadResponse)
	}
	b := make([]byte, length)
	if _, err = io.ReadFull(r, b); err != nil {
		return nil, minecraftProblem(ctx, err)
	}
	frame := bytes.NewReader(b)
	id, err := readVarInt(frame)
	if err != nil || id != 0 {
		return nil, dto.Failure(dto.BadResponse)
	}
	size, err := readVarInt(frame)
	if err != nil || size != frame.Len() || size > minecraftLimit {
		return nil, dto.Failure(dto.BadResponse)
	}
	payload := make([]byte, size)
	_, _ = io.ReadFull(frame, payload)
	var status struct {
		Version *struct {
			Name *string `json:"name"`
		} `json:"version"`
		Players *struct {
			Online *int `json:"online"`
			Max    *int `json:"max"`
			Sample []struct {
				Name *string `json:"name"`
			} `json:"sample"`
		} `json:"players"`
		Description json.RawMessage `json:"description"`
	}
	if !utf8.Valid(payload) || json.Unmarshal(payload, &status) != nil || status.Version == nil || status.Version.Name == nil || status.Players == nil || status.Players.Online == nil || status.Players.Max == nil || *status.Players.Online < 0 || *status.Players.Max < 0 || len(status.Description) == 0 {
		return nil, dto.Failure(dto.BadResponse)
	}
	var component any
	if json.Unmarshal(status.Description, &component) != nil {
		return nil, dto.Failure(dto.BadResponse)
	}
	motd, ok := flatten(component, 0)
	if !ok {
		return nil, dto.Failure(dto.BadResponse)
	}
	out := &dto.MinecraftData{Online: count(*status.Players.Online), Max: count(*status.Players.Max), Version: text(*status.Version.Name), MOTD: stripFormatting(motd), Players: []string{}}
	for _, p := range status.Players.Sample[:min(12, len(status.Players.Sample))] {
		if p.Name == nil {
			return nil, dto.Failure(dto.BadResponse)
		}
		out.Players = append(out.Players, stripFormatting(*p.Name))
	}
	return out, nil
}

// Status is independent of the gameplay protocol revision; modern Java servers
// answer this negotiation before entering the version-specific login state.
const mathProtocolVersion = 767

func minecraftProblem(ctx context.Context, err error) *dto.Problem {
	p := transportProblem(ctx, err)
	if p.Code == dto.Timeout || ctx.Err() != nil {
		return p
	}
	return dto.Failure(dto.BadResponse)
}
func flatten(v any, depth int) (string, bool) {
	if depth > 32 {
		return "", false
	}
	switch x := v.(type) {
	case string:
		return text(x), true
	case []any:
		var b strings.Builder
		for _, part := range x {
			s, ok := flatten(part, depth+1)
			if !ok {
				return "", false
			}
			b.WriteString(s)
			if b.Len() > 2048 {
				break
			}
		}
		return text(b.String()), true
	case map[string]any:
		base := ""
		valid := false
		if t, exists := x["text"]; exists {
			var ok bool
			base, ok = t.(string)
			if !ok {
				return "", false
			}
			valid = true
		}
		if extra, exists := x["extra"]; exists {
			parts, ok := extra.([]any)
			if !ok {
				return "", false
			}
			s, ok := flatten(parts, depth+1)
			if !ok {
				return "", false
			}
			base += s
			valid = true
		}
		return text(base), valid
	}
	return "", false
}
func stripFormatting(s string) string {
	r := []rune(s)
	out := make([]rune, 0, len(r))
	for i := 0; i < len(r); i++ {
		if r[i] == '§' {
			if i+1 < len(r) {
				i++
			}
			continue
		}
		out = append(out, r[i])
	}
	return text(string(out))
}
