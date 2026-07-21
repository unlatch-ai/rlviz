// Package tea is a deliberately small, offline-buildable subset of the
// Bubble Tea runtime. It preserves the Model/Update/View contract used by
// RLViz without bringing networking or a large terminal dependency graph into
// the viewer binary.
package tea

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type Msg any
type Cmd func() Msg

type Model interface {
	Init() Cmd
	Update(Msg) (Model, Cmd)
	View() string
}

type KeyType int

const (
	KeyRunes KeyType = iota
	KeyEnter
	KeyEsc
	KeyCtrlC
	KeyUp
	KeyDown
)

type KeyMsg struct {
	Type  KeyType
	Runes []rune
}

func (key KeyMsg) String() string {
	switch key.Type {
	case KeyEnter:
		return "enter"
	case KeyEsc:
		return "esc"
	case KeyCtrlC:
		return "ctrl+c"
	case KeyUp:
		return "up"
	case KeyDown:
		return "down"
	default:
		return string(key.Runes)
	}
}

type WindowSizeMsg struct{ Width, Height int }
type quitMsg struct{}

func Quit() Msg { return quitMsg{} }

type ProgramOption func(*Program)

func WithAltScreen() ProgramOption { return func(program *Program) { program.altScreen = true } }
func WithInput(input io.Reader) ProgramOption {
	return func(program *Program) { program.input = input }
}
func WithOutput(output io.Writer) ProgramOption {
	return func(program *Program) { program.output = output }
}

type Program struct {
	model     Model
	input     io.Reader
	output    io.Writer
	altScreen bool
}

func NewProgram(model Model, options ...ProgramOption) *Program {
	program := &Program{model: model, input: os.Stdin, output: os.Stdout}
	for _, option := range options {
		option(program)
	}
	return program
}

func (program *Program) Run() (Model, error) {
	terminal, initialWidth, initialHeight := false, 0, 0
	if file, ok := program.input.(*os.File); ok {
		initialWidth, initialHeight, terminal = terminalSize(file)
	}
	if terminal {
		command := exec.Command("stty", "raw", "opost", "-echo")
		command.Stdin = program.input
		_ = command.Run()
		defer func() { restore := exec.Command("stty", "sane"); restore.Stdin = program.input; _ = restore.Run() }()
	}
	if program.altScreen && terminal {
		fmt.Fprint(program.output, "\x1b[?1049h")
		defer fmt.Fprint(program.output, "\x1b[?1049l")
	}
	if command := program.model.Init(); command != nil {
		program.model, _ = program.model.Update(command())
	}
	var resize <-chan os.Signal
	if terminal {
		program.model, _ = program.model.Update(WindowSizeMsg{Width: initialWidth, Height: initialHeight})
		resizes := make(chan os.Signal, 1)
		signal.Notify(resizes, syscall.SIGWINCH)
		defer signal.Stop(resizes)
		resize = resizes
	}
	program.render(terminal)
	reader := newKeyReader(program.input)
	type keyResult struct {
		key KeyMsg
		err error
	}
	keys := make(chan keyResult)
	go func() {
		for {
			key, err := reader.readKey()
			keys <- keyResult{key: key, err: err}
			if err != nil {
				return
			}
		}
	}()
	for {
		select {
		case result := <-keys:
			if result.err == io.EOF {
				return program.model, nil
			}
			if result.err != nil {
				return program.model, result.err
			}
			next, command := program.model.Update(result.key)
			program.model = next
			if command != nil {
				if _, ok := command().(quitMsg); ok {
					return program.model, nil
				}
			}
			program.render(terminal)
		case <-resize:
			if width, height, ok := terminalSize(program.input); ok {
				program.model, _ = program.model.Update(WindowSizeMsg{Width: width, Height: height})
				program.render(terminal)
			}
		}
	}
}

func terminalSize(input io.Reader) (int, int, bool) {
	command := exec.Command("stty", "size")
	command.Stdin = input
	output, err := command.Output()
	if err != nil {
		return 0, 0, false
	}
	fields := strings.Fields(string(output))
	if len(fields) != 2 {
		return 0, 0, false
	}
	height, heightErr := strconv.Atoi(fields[0])
	width, widthErr := strconv.Atoi(fields[1])
	return width, height, heightErr == nil && widthErr == nil && width > 0 && height > 0
}

func (program *Program) render(terminal bool) {
	if terminal {
		fmt.Fprint(program.output, "\x1b[H\x1b[2J")
	}
	fmt.Fprint(program.output, program.model.View())
	if !terminal {
		fmt.Fprintln(program.output)
	}
}

type inputByte struct {
	value byte
	err   error
}

type keyReader struct {
	input   <-chan inputByte
	pending []byte
}

func newKeyReader(input io.Reader) *keyReader {
	bytes := make(chan inputByte, 64)
	go func() {
		defer close(bytes)
		buffer := make([]byte, 64)
		for {
			count, err := input.Read(buffer)
			for _, value := range buffer[:count] {
				bytes <- inputByte{value: value}
			}
			if err != nil {
				bytes <- inputByte{err: err}
				return
			}
		}
	}()
	return &keyReader{input: bytes}
}

func (reader *keyReader) nextByte(wait time.Duration) (byte, error, bool) {
	if len(reader.pending) > 0 {
		value := reader.pending[0]
		reader.pending = reader.pending[1:]
		return value, nil, true
	}
	if wait <= 0 {
		item, ok := <-reader.input
		if !ok {
			return 0, io.EOF, true
		}
		return item.value, item.err, true
	}
	select {
	case item, ok := <-reader.input:
		if !ok {
			return 0, io.EOF, true
		}
		return item.value, item.err, true
	case <-time.After(wait):
		return 0, nil, false
	}
}

func (reader *keyReader) readKey() (KeyMsg, error) {
	value, err, _ := reader.nextByte(0)
	if err != nil {
		return KeyMsg{}, err
	}
	switch value {
	case 3:
		return KeyMsg{Type: KeyCtrlC}, nil
	case '\r', '\n':
		return KeyMsg{Type: KeyEnter}, nil
	case 27:
		second, secondErr, ok := reader.nextByte(25 * time.Millisecond)
		if secondErr != nil || !ok {
			return KeyMsg{Type: KeyEsc}, nil
		}
		if second != '[' {
			reader.pending = append([]byte{second}, reader.pending...)
			return KeyMsg{Type: KeyEsc}, nil
		}
		for count := 0; count < 32; count++ {
			part, partErr, available := reader.nextByte(25 * time.Millisecond)
			if partErr != nil || !available {
				return reader.readKey()
			}
			if part >= 0x40 && part <= 0x7e {
				switch part {
				case 'A':
					return KeyMsg{Type: KeyUp}, nil
				case 'B':
					return KeyMsg{Type: KeyDown}, nil
				default:
					return reader.readKey()
				}
			}
		}
		return reader.readKey()
	default:
		return KeyMsg{Type: KeyRunes, Runes: []rune{rune(value)}}, nil
	}
}
