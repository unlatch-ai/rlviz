package tea

import (
	"io"
	"testing"
	"time"
)

func TestReadKeyConsumesSplitAndParameterizedCSI(t *testing.T) {
	inputReader, inputWriter := io.Pipe()
	reader := newKeyReader(inputReader)
	result := make(chan KeyMsg, 1)
	go func() {
		key, _ := reader.readKey()
		result <- key
	}()
	_, _ = inputWriter.Write([]byte("\x1b[1;"))
	time.Sleep(5 * time.Millisecond)
	_, _ = inputWriter.Write([]byte("5A"))
	if key := <-result; key.Type != KeyUp {
		t.Fatalf("key = %#v, want up", key)
	}
	_ = inputWriter.Close()
}

func TestReadKeySwallowsUnknownCSI(t *testing.T) {
	reader := newKeyReader(&chunkReader{chunks: [][]byte{[]byte("\x1b[3~"), []byte("j")}})
	key, err := reader.readKey()
	if err != nil || key.String() != "j" {
		t.Fatalf("key=%#v err=%v, want j after swallowed CSI", key, err)
	}
}

type chunkReader struct{ chunks [][]byte }

func (reader *chunkReader) Read(target []byte) (int, error) {
	if len(reader.chunks) == 0 {
		return 0, io.EOF
	}
	chunk := reader.chunks[0]
	reader.chunks = reader.chunks[1:]
	return copy(target, chunk), nil
}
