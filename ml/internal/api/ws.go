package api

import (
	"bufio"
	"crypto/sha1"
	"encoding/base64"
	"fmt"
	"net"
	"net/http"
	"strings"
)

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// wsUpgrade realiza el handshake y devuelve la conexión TCP cruda.
func wsUpgrade(w http.ResponseWriter, r *http.Request) (net.Conn, error) {
	if !strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
		return nil, fmt.Errorf("no es una petición de upgrade websocket")
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		return nil, fmt.Errorf("falta Sec-WebSocket-Key")
	}
	h := sha1.Sum([]byte(key + wsGUID))
	accept := base64.StdEncoding.EncodeToString(h[:])

	hj, ok := w.(http.Hijacker)
	if !ok {
		return nil, fmt.Errorf("el servidor no soporta hijacking")
	}
	conn, buf, err := hj.Hijack()
	if err != nil {
		return nil, err
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + accept + "\r\n\r\n"
	if _, err := buf.WriteString(resp); err != nil {
		conn.Close()
		return nil, err
	}
	if err := buf.Flush(); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

// wsWriteText envía un frame de texto (opcode 0x1, FIN=1) sin máscara
// (los frames servidor→cliente no se enmascaran según el RFC).
func wsWriteText(conn net.Conn, msg []byte) error {
	n := len(msg)
	var header []byte
	switch {
	case n < 126:
		header = []byte{0x81, byte(n)}
	case n <= 0xFFFF:
		header = []byte{0x81, 126, byte(n >> 8), byte(n)}
	default:
		header = []byte{0x81, 127,
			byte(uint64(n) >> 56), byte(uint64(n) >> 48), byte(uint64(n) >> 40), byte(uint64(n) >> 32),
			byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)}
	}
	if _, err := conn.Write(header); err != nil {
		return err
	}
	_, err := conn.Write(msg)
	return err
}

// wsDrain lee y descarta los frames entrantes del cliente para detectar
// el cierre de la conexión (opcode 0x8) y liberar recursos.
func wsDrain(conn net.Conn, done chan<- struct{}) {
	r := bufio.NewReader(conn)
	defer close(done)
	for {
		b1, err := r.ReadByte()
		if err != nil {
			return
		}
		opcode := b1 & 0x0F
		b2, err := r.ReadByte()
		if err != nil {
			return
		}
		masked := b2&0x80 != 0
		length := int64(b2 & 0x7F)
		switch length {
		case 126:
			var ext [2]byte
			if _, err := ioReadFull(r, ext[:]); err != nil {
				return
			}
			length = int64(ext[0])<<8 | int64(ext[1])
		case 127:
			var ext [8]byte
			if _, err := ioReadFull(r, ext[:]); err != nil {
				return
			}
			length = 0
			for _, b := range ext {
				length = length<<8 | int64(b)
			}
		}
		if masked {
			var mask [4]byte
			if _, err := ioReadFull(r, mask[:]); err != nil {
				return
			}
		}
		// descartar payload
		if length > 0 {
			if _, err := r.Discard(int(length)); err != nil {
				return
			}
		}
		if opcode == 0x8 { // close frame
			return
		}
	}
}

func ioReadFull(r *bufio.Reader, buf []byte) (int, error) {
	total := 0
	for total < len(buf) {
		n, err := r.Read(buf[total:])
		total += n
		if err != nil {
			return total, err
		}
	}
	return total, nil
}
