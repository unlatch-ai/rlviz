// Package tea is a deliberately small, offline-buildable subset of the
// Bubble Tea runtime. It preserves the Model/Update/View contract used by
// RLViz without bringing networking or a large terminal dependency graph into
// the viewer binary.
package tea

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"

	"github.com/mattn/go-isatty"
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
	terminal := false
	if file, ok := program.input.(*os.File); ok {
		terminal = isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
	}
	if terminal {
		command := exec.Command("stty", "raw", "-echo")
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
	program.render(terminal)
	reader := bufio.NewReader(program.input)
	for {
		key, err := readKey(reader)
		if err == io.EOF {
			return program.model, nil
		}
		if err != nil {
			return program.model, err
		}
		next, command := program.model.Update(key)
		program.model = next
		if command != nil {
			if _, ok := command().(quitMsg); ok {
				return program.model, nil
			}
		}
		program.render(terminal)
	}
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

func readKey(reader *bufio.Reader) (KeyMsg, error) {
	value, err := reader.ReadByte()
	if err != nil {
		return KeyMsg{}, err
	}
	switch value {
	case 3:
		return KeyMsg{Type: KeyCtrlC}, nil
	case '\r', '\n':
		return KeyMsg{Type: KeyEnter}, nil
	case 27:
		if reader.Buffered() >= 2 {
			second, _ := reader.ReadByte()
			third, _ := reader.ReadByte()
			if second == '[' && third == 'A' {
				return KeyMsg{Type: KeyUp}, nil
			}
			if second == '[' && third == 'B' {
				return KeyMsg{Type: KeyDown}, nil
			}
		}
		return KeyMsg{Type: KeyEsc}, nil
	default:
		return KeyMsg{Type: KeyRunes, Runes: []rune{rune(value)}}, nil
	}
}
