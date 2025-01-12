package multiplex

import (
	"bytes"
	"github.com/cbeuw/Cloak/internal/util"
	"math/rand"
	"net"
	"testing"
	"time"
)

func setupSesh(unordered bool) *Session {
	sessionKey := make([]byte, 32)
	rand.Read(sessionKey)
	obfuscator, _ := GenerateObfs(0x00, sessionKey, true)

	seshConfig := &SessionConfig{
		Obfuscator: obfuscator,
		Valve:      nil,
		UnitRead:   util.ReadTLS,
		Unordered:  unordered,
	}
	return MakeSession(0, seshConfig)
}

func BenchmarkStream_Write_Ordered(b *testing.B) {
	const PAYLOAD_LEN = 1000
	hole := newBlackHole()
	sesh := setupSesh(false)
	sesh.AddConnection(hole)
	testData := make([]byte, PAYLOAD_LEN)
	rand.Read(testData)

	stream, _ := sesh.OpenStream()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := stream.Write(testData)
		if err != nil {
			b.Error(
				"For", "stream write",
				"got", err,
			)
		}
		b.SetBytes(PAYLOAD_LEN)
	}
}

func BenchmarkStream_Read_Ordered(b *testing.B) {
	sesh := setupSesh(false)
	const PAYLOAD_LEN = 1000
	testPayload := make([]byte, PAYLOAD_LEN)
	rand.Read(testPayload)

	f := &Frame{
		1,
		0,
		0,
		testPayload,
	}

	obfsBuf := make([]byte, 17000)

	l, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		// potentially bottlenecked here rather than the actual stream read throughput
		conn, _ := net.Dial("tcp", l.Addr().String())
		for {
			i, _ := sesh.Obfs(f, obfsBuf)
			f.Seq += 1
			_, err := conn.Write(obfsBuf[:i])
			if err != nil {
				b.Error("cannot write to connection", err)
			}
		}
	}()
	conn, _ := l.Accept()

	sesh.AddConnection(conn)
	stream, err := sesh.Accept()
	if err != nil {
		b.Error("failed to accept stream", err)
	}

	//time.Sleep(5*time.Second) // wait for buffer to fill up

	readBuf := make([]byte, PAYLOAD_LEN)
	b.ResetTimer()
	for j := 0; j < b.N; j++ {
		n, err := stream.Read(readBuf)
		if !bytes.Equal(readBuf, testPayload) {
			b.Error("paylod not equal")
		}
		b.SetBytes(int64(n))
		if err != nil {
			b.Error(err)
		}
	}

}

func TestStream_Write(t *testing.T) {
	const PAYLOAD_LEN = 1000
	hole := newBlackHole()
	sesh := setupSesh(false)
	sesh.AddConnection(hole)
	testData := make([]byte, PAYLOAD_LEN)
	rand.Read(testData)

	stream, _ := sesh.OpenStream()
	_, err := stream.Write(testData)
	if err != nil {
		t.Error(
			"For", "stream write",
			"got", err,
		)
	}
}

func TestStream_Close(t *testing.T) {
	sesh := setupSesh(false)
	testPayload := []byte{42, 42, 42}
	streamID := uint32(1)

	f := &Frame{
		streamID,
		0,
		0,
		testPayload,
	}
	ch := make(chan []byte)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		conn, _ := net.Dial("tcp", l.Addr().String())
		for {
			data := <-ch
			_, err := conn.Write(data)
			if err != nil {
				t.Error("cannot write to connection", err)
				return
			}
		}
	}()
	conn, _ := l.Accept()
	sesh.AddConnection(conn)
	obfsBuf := make([]byte, 512)
	i, _ := sesh.Obfs(f, obfsBuf)
	ch <- obfsBuf[:i]
	time.Sleep(100 * time.Microsecond)
	stream, err := sesh.Accept()
	if err != nil {
		t.Error("failed to accept stream", err)
		return
	}
	err = stream.Close()
	if err != nil {
		t.Error("failed to actively close stream", err)
		return
	}

	if _, ok := sesh.streams[streamID]; ok {
		t.Error("stream still exists")
		return
	}
}

func TestStream_Read(t *testing.T) {
	sesh := setupSesh(false)
	testPayload := []byte{42, 42, 42}
	const PAYLOAD_LEN = 3

	f := &Frame{
		1,
		0,
		0,
		testPayload,
	}

	ch := make(chan []byte)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		conn, _ := net.Dial("tcp", l.Addr().String())
		for {
			data := <-ch
			_, err := conn.Write(data)
			if err != nil {
				t.Error("cannot write to connection", err)
				return
			}
		}
	}()
	conn, _ := l.Accept()
	sesh.AddConnection(conn)

	var streamID uint32
	buf := make([]byte, 10)

	obfsBuf := make([]byte, 512)
	t.Run("Plain read", func(t *testing.T) {
		f.StreamID = streamID
		i, _ := sesh.Obfs(f, obfsBuf)
		streamID++
		ch <- obfsBuf[:i]
		time.Sleep(100 * time.Microsecond)
		stream, err := sesh.Accept()
		if err != nil {
			t.Error("failed to accept stream", err)
			return
		}
		i, err = stream.Read(buf)
		if err != nil {
			t.Error("failed to read", err)
			return
		}
		if i != PAYLOAD_LEN {
			t.Errorf("expected read %v, got %v", PAYLOAD_LEN, i)
			return
		}
		if !bytes.Equal(buf[:i], testPayload) {
			t.Error("expected", testPayload,
				"got", buf[:i])
			return
		}
	})
	t.Run("Nil buf", func(t *testing.T) {
		f.StreamID = streamID
		i, _ := sesh.Obfs(f, obfsBuf)
		streamID++
		ch <- obfsBuf[:i]
		time.Sleep(100 * time.Microsecond)
		stream, _ := sesh.Accept()
		i, err := stream.Read(nil)
		if i != 0 || err != nil {
			t.Error("expecting", 0, nil,
				"got", i, err)
		}

		stream.Close()
		i, err = stream.Read(nil)
		if i != 0 || err != ErrBrokenStream {
			t.Error("expecting", 0, ErrBrokenStream,
				"got", i, err)
		}

	})
	t.Run("Read after stream close", func(t *testing.T) {
		f.StreamID = streamID
		i, _ := sesh.Obfs(f, obfsBuf)
		streamID++
		ch <- obfsBuf[:i]
		time.Sleep(100 * time.Microsecond)
		stream, _ := sesh.Accept()
		stream.Close()
		i, err := stream.Read(buf)
		if err != nil {
			t.Error("failed to read", err)
		}
		if i != PAYLOAD_LEN {
			t.Errorf("expected read %v, got %v", PAYLOAD_LEN, i)
		}
		if !bytes.Equal(buf[:i], testPayload) {
			t.Error("expected", testPayload,
				"got", buf[:i])
		}
		_, err = stream.Read(buf)
		if err == nil {
			t.Error("expecting error", ErrBrokenStream,
				"got nil error")
		}
	})
	t.Run("Read after session close", func(t *testing.T) {
		f.StreamID = streamID
		i, _ := sesh.Obfs(f, obfsBuf)
		streamID++
		ch <- obfsBuf[:i]
		time.Sleep(100 * time.Microsecond)
		stream, _ := sesh.Accept()
		sesh.Close()
		i, err := stream.Read(buf)
		if err != nil {
			t.Error("failed to read", err)
		}
		if i != PAYLOAD_LEN {
			t.Errorf("expected read %v, got %v", PAYLOAD_LEN, i)
		}
		if !bytes.Equal(buf[:i], testPayload) {
			t.Error("expected", testPayload,
				"got", buf[:i])
		}
		_, err = stream.Read(buf)
		if err == nil {
			t.Error("expecting error", ErrBrokenStream,
				"got nil error")
		}
	})

}

func TestStream_UnorderedRead(t *testing.T) {
	sesh := setupSesh(true)
	testPayload := []byte{42, 42, 42}
	const PAYLOAD_LEN = 3

	f := &Frame{
		1,
		0,
		0,
		testPayload,
	}

	ch := make(chan []byte)
	l, _ := net.Listen("tcp", "127.0.0.1:0")
	go func() {
		conn, _ := net.Dial("tcp", l.Addr().String())
		for {
			data := <-ch
			_, err := conn.Write(data)
			if err != nil {
				t.Error("cannot write to connection", err)
			}
		}
	}()
	conn, _ := l.Accept()
	sesh.AddConnection(conn)

	var streamID uint32
	buf := make([]byte, 10)

	obfsBuf := make([]byte, 512)
	t.Run("Plain read", func(t *testing.T) {
		f.StreamID = streamID
		i, _ := sesh.Obfs(f, obfsBuf)
		streamID++
		ch <- obfsBuf[:i]
		time.Sleep(100 * time.Microsecond)
		stream, err := sesh.Accept()
		if err != nil {
			t.Error("failed to accept stream", err)
		}
		i, err = stream.Read(buf)
		if err != nil {
			t.Error("failed to read", err)
		}
		if i != PAYLOAD_LEN {
			t.Errorf("expected read %v, got %v", PAYLOAD_LEN, i)
		}
		if !bytes.Equal(buf[:i], testPayload) {
			t.Error("expected", testPayload,
				"got", buf[:i])
		}
	})
	t.Run("Nil buf", func(t *testing.T) {
		f.StreamID = streamID
		i, _ := sesh.Obfs(f, obfsBuf)
		streamID++
		ch <- obfsBuf[:i]
		time.Sleep(100 * time.Microsecond)
		stream, _ := sesh.Accept()
		i, err := stream.Read(nil)
		if i != 0 || err != nil {
			t.Error("expecting", 0, nil,
				"got", i, err)
		}

		stream.Close()
		i, err = stream.Read(nil)
		if i != 0 || err != ErrBrokenStream {
			t.Error("expecting", 0, ErrBrokenStream,
				"got", i, err)
		}

	})
	t.Run("Read after stream close", func(t *testing.T) {
		f.StreamID = streamID
		i, _ := sesh.Obfs(f, obfsBuf)
		streamID++
		ch <- obfsBuf[:i]
		time.Sleep(100 * time.Microsecond)
		stream, _ := sesh.Accept()
		stream.Close()
		i, err := stream.Read(buf)
		if err != nil {
			t.Error("failed to read", err)
		}
		if i != PAYLOAD_LEN {
			t.Errorf("expected read %v, got %v", PAYLOAD_LEN, i)
		}
		if !bytes.Equal(buf[:i], testPayload) {
			t.Error("expected", testPayload,
				"got", buf[:i])
		}
		_, err = stream.Read(buf)
		if err == nil {
			t.Error("expecting error", ErrBrokenStream,
				"got nil error")
		}
	})
	t.Run("Read after session close", func(t *testing.T) {
		f.StreamID = streamID
		i, _ := sesh.Obfs(f, obfsBuf)
		streamID++
		ch <- obfsBuf[:i]
		time.Sleep(100 * time.Microsecond)
		stream, _ := sesh.Accept()
		sesh.Close()
		i, err := stream.Read(buf)
		if err != nil {
			t.Error("failed to read", err)
		}
		if i != PAYLOAD_LEN {
			t.Errorf("expected read %v, got %v", PAYLOAD_LEN, i)
		}
		if !bytes.Equal(buf[:i], testPayload) {
			t.Error("expected", testPayload,
				"got", buf[:i])
		}
		_, err = stream.Read(buf)
		if err == nil {
			t.Error("expecting error", ErrBrokenStream,
				"got nil error")
		}
	})

}
