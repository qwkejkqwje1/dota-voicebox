package ui

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/qwkejkqwje1/dota-voicebox/internal/audio"
)

// Минимальный WebSocket (RFC 6455) — только то, что нужно для потока визуализации.

type wsConn struct {
	c  net.Conn
	rw *bufio.ReadWriter
}

func wsUpgrade(w http.ResponseWriter, r *http.Request) (*wsConn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, errors.New("not websocket")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	h, ok := w.(http.Hijacker)
	if !ok || key == "" {
		return nil, errors.New("bad request")
	}
	c, rw, err := h.Hijack()
	if err != nil {
		return nil, err
	}
	sum := sha1.Sum([]byte(key + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
	rw.WriteString("HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: " +
		base64.StdEncoding.EncodeToString(sum[:]) + "\r\n\r\n")
	if err := rw.Flush(); err != nil {
		c.Close()
		return nil, err
	}
	return &wsConn{c: c, rw: rw}, nil
}

func (ws *wsConn) send(op byte, p []byte) error {
	hdr := []byte{0x80 | op}
	switch n := len(p); {
	case n < 126:
		hdr = append(hdr, byte(n))
	case n < 65536:
		hdr = append(hdr, 126, byte(n>>8), byte(n))
	default:
		hdr = append(hdr, 127, 0, 0, 0, 0, byte(n>>24), byte(n>>16), byte(n>>8), byte(n))
	}
	ws.c.SetWriteDeadline(time.Now().Add(2 * time.Second))
	if _, err := ws.rw.Write(hdr); err != nil {
		return err
	}
	if _, err := ws.rw.Write(p); err != nil {
		return err
	}
	return ws.rw.Flush()
}

// read читает одно сообщение клиента (кадры клиента всегда замаскированы).
func (ws *wsConn) read() (op byte, msg []byte, err error) {
	var h [2]byte
	if _, err = io.ReadFull(ws.rw, h[:]); err != nil {
		return
	}
	op = h[0] & 0x0f
	n := uint64(h[1] & 0x7f)
	switch n {
	case 126:
		var b [2]byte
		if _, err = io.ReadFull(ws.rw, b[:]); err != nil {
			return
		}
		n = uint64(binary.BigEndian.Uint16(b[:]))
	case 127:
		var b [8]byte
		if _, err = io.ReadFull(ws.rw, b[:]); err != nil {
			return
		}
		n = binary.BigEndian.Uint64(b[:])
	}
	if n > 1<<16 {
		return 0, nil, errors.New("too big")
	}
	var mask [4]byte
	if h[1]&0x80 != 0 {
		if _, err = io.ReadFull(ws.rw, mask[:]); err != nil {
			return
		}
	}
	msg = make([]byte, n)
	if _, err = io.ReadFull(ws.rw, msg); err != nil {
		return
	}
	for i := range msg {
		msg[i] ^= mask[i%4]
	}
	return
}

// viz — поток визуализации голоса: ~30 кадров/с, бинарные кадры.
// Клиент присылает текст "pre" / "post" (до/после эффектов) и "fps:NN".
func (s *Server) viz(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("t") != s.Token {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	ws, err := wsUpgrade(w, r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer ws.c.Close()
	var post atomic.Bool
	var fps atomic.Int32
	fps.Store(30)
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			op, msg, err := ws.read()
			if err != nil || op == 8 {
				return
			}
			switch m := string(msg); {
			case m == "pre":
				post.Store(false)
			case m == "post":
				post.Store(true)
			case strings.HasPrefix(m, "fps:"):
				var n int32
				for _, c := range m[4:] {
					n = n*10 + int32(c-'0')
				}
				if n >= 5 && n <= 60 {
					fps.Store(n)
				}
			}
		}
	}()
	v := audio.NewViz(2048, 256, 96)
	out := make([]byte, 256*2+96+1+2)
	for {
		select {
		case <-done:
			return
		case <-time.After(time.Second / time.Duration(fps.Load())):
		}
		s.Engine.VizSnapshot(v.Samples(), post.Load())
		n := v.Frame(out[2:])
		out[0] = 1 // версия формата
		out[1] = 0
		if s.Engine.GateOpen() {
			out[1] = 1
		}
		if err := ws.send(2, out[:n+2]); err != nil {
			return
		}
	}
}
