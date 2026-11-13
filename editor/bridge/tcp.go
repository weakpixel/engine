package bridge

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net"
	"sync"
	"time"

	"github.com/weakpixel/engine/editor"
)

// Environment variables a native host sets to connect a game to itself (see
// [ConnectHost]). They are read on every platform except the browser, where
// the page's script is the host. The variables that
// name the stage, the first file and headless mode are in package hostlink.
const (
	// EnvBridge is the address (host:port) of the host's listener.
	EnvBridge = "WEAKPIXEL_BRIDGE"
	// EnvBridgeToken is the secret the game sends as the first line.
	EnvBridgeToken = "WEAKPIXEL_BRIDGE_TOKEN"
)

// maxLine caps one message line, a level document included.
const maxLine = 64 << 20

// writeTimeout bounds one message write, so a host that stopped reading
// ends the game instead of blocking its tick forever.
const writeTimeout = 10 * time.Second

// DialHost connects to a host listening on the TCP address addr
// ("127.0.0.1:port") and returns a bridge to it. The protocol is the same as
// in the browser; only the transport differs: once the connection is open
// the game sends token and a newline, then both sides exchange one JSON
// message per line (a JSON text never holds a raw newline). When the host
// closes the connection the bridge quits the engine at its next tick, so a
// game started by a host ends with it.
func DialHost(addr, token string, cfg editor.Config) (*Bridge, error) {
	conn, err := net.DialTimeout("tcp", addr, 5*time.Second)
	if err != nil {
		return nil, fmt.Errorf("bridge: connect to the host: %w", err)
	}
	t := &tcpTransport{conn: conn}
	b := New(cfg, t)
	t.b = b
	if _, err := io.WriteString(conn, token+"\n"); err != nil {
		conn.Close()
		return nil, fmt.Errorf("bridge: send the token: %w", err)
	}
	go t.read()
	return b, nil
}

// Close closes the connection to the host, if the bridge has one.
func (b *Bridge) Close() error {
	if c, ok := b.t.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

// tcpTransport is the Transport of a bridge connected with [DialHost].
type tcpTransport struct {
	conn net.Conn
	b    *Bridge
	mu   sync.Mutex // one write at a time
}

// Send writes msg and a newline. A failed write marks the host gone.
func (t *tcpTransport) Send(msg []byte) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.conn.SetWriteDeadline(time.Now().Add(writeTimeout))
	buf := make([]byte, 0, len(msg)+1)
	buf = append(append(buf, msg...), '\n')
	if _, err := t.conn.Write(buf); err != nil {
		t.b.gone.Store(true)
	}
}

func (t *tcpTransport) Close() error { return t.conn.Close() }

// read queues every line the host sends until the connection ends.
func (t *tcpTransport) read() {
	defer t.b.gone.Store(true)
	r := bufio.NewReaderSize(t.conn, 64<<10)
	for {
		line, err := readLine(r)
		if len(line) > 0 {
			t.b.Receive(line)
		}
		if err != nil {
			return
		}
	}
}

// readLine reads one message line from r without its line end, joining the
// pieces of a line longer than the reader's buffer. It fails when the line
// is over 64 MiB. The last line of a stream that ends without a newline is
// returned together with io.EOF.
func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		part, err := r.ReadSlice('\n')
		line = append(line, part...)
		if len(line) > maxLine {
			return nil, errors.New("bridge: message line is too long")
		}
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return bytes.TrimRight(line, "\r\n"), err
	}
}
