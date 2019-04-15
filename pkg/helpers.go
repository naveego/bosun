package pkg

import (
	"bufio"
	"errors"
	"fmt"
	"github.com/fatih/color"
	"github.com/manifoldco/promptui"
	"github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"io"
	"io/ioutil"
	"os"
	"runtime"
	"strings"
)

var Log = logrus.NewEntry(logrus.StandardLogger())

func RequestConfirmFromUser(label string, args ... interface{}) bool {

	if !IsInteractive() {
		return false
	}

	prompt := promptui.Prompt{
		Label: fmt.Sprintf(label, args...) + " [y/N]",
	}

	value, err := prompt.Run()
	if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
		fmt.Println("User quit.")
		os.Exit(0)
	}

	if strings.HasPrefix(strings.ToLower(value), "y") {
		return true
	}

	return false
}

func RequestStringFromUser(text string, args ... interface{}) (string) {

	if !IsInteractive() {
		panic(fmt.Sprintf("Requested string from user but no terminal is attached: %q, %v", text, args))
	}

	prompt := promptui.Prompt{
		Label: fmt.Sprintf(text, args...),
	}

	value, err := prompt.Run()
	if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
		fmt.Println("User quit.")
		os.Exit(0)
	}

	return value
}

func RequestSecretFromUser(text string, args ... interface{}) (string) {
	prompt := promptui.Prompt{
		Label: fmt.Sprintf(text, args...),
		Mask:  '*',
	}

	value, err := prompt.Run()
	if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
		fmt.Println("User quit.")
		os.Exit(0)
	}

	return value
}


// IfTTY invokes the function if there is a TTY attached, and returns ErrNoTTY otherwise.
// If the function returns an error of promptui.ErrInterrupt or promptui.ErrAbort
// this will call os.Exit.
func IfTTY(fn func() (string, error)) (string, error) {
	if !IsInteractive() {
		return "", ErrNoTTY
	}

	result, err := fn()
	if err == promptui.ErrInterrupt || err == promptui.ErrAbort {
		fmt.Println("User quit.")
		os.Exit(0)
	}

	return result, err
}

var ErrNoTTY = errors.New("no tty attached")

func Must(err error, msgAndArgs ... string) {
	if err == nil {
		return
	}
	var msg string
	switch len(msgAndArgs) {
	case 0:
		msg = "Fatal error."
	case 1:
		msg = msgAndArgs[0]
	default:
		msg = fmt.Sprintf(msgAndArgs[0], msgAndArgs[1:])
	}

	color.Red(msg)
	color.Yellow(err.Error())

	_, file, line, ok := runtime.Caller(1)
	if ok {
		color.Blue("@ %s : line %d", file, line)
	}

	os.Exit(1)
}

func LoadYaml(path string, out interface{}) error {
	b, err := ioutil.ReadFile(path)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(b, out)
	return err
}


// Prefixer implements io.Reader and io.WriterTo. It reads
// data from the underlying reader and prepends every line
// with a given string.
type Prefixer struct {
	reader *bufio.Reader
	prefix []byte
	unread []byte
	eof    bool
}

// New creates a new instance of Prefixer.
func New(r io.Reader, prefix string) *Prefixer {
	return &Prefixer{
		reader: bufio.NewReader(r),
		prefix: []byte(prefix),
	}
}

// Read implements io.Reader. It reads data into p from the
// underlying reader and prepends every line with a prefix.
// It does not block if no data is available yet.
// It returns the number of bytes read into p.
func (r *Prefixer) Read(p []byte) (n int, err error) {
	for {
		// Write unread data from previous read.
		if len(r.unread) > 0 {
			m := copy(p[n:], r.unread)
			n += m
			r.unread = r.unread[m:]
			if len(r.unread) > 0 {
				return n, nil
			}
		}

		// The underlying Reader already returned EOF, do not read again.
		if r.eof {
			return n, io.EOF
		}

		// Read new line, including delim.
		r.unread, err = r.reader.ReadBytes('\n')

		if err == io.EOF {
			r.eof = true
		}

		// No new data, do not block.
		if len(r.unread) == 0 {
			return n, err
		}

		// Some new data, prepend prefix.
		// TODO: We could write the prefix to r.unread buffer just once
		//       and re-use it instead of prepending every time.
		r.unread = append(r.prefix, r.unread...)

		if err != nil {
			if err == io.EOF && len(r.unread) > 0 {
				// The underlying Reader already returned EOF, but we still
				// have some unread data to send, thus clear the error.
				return n, nil
			}
			return n, err
		}
	}
	panic("unreachable")
}

func (r *Prefixer) WriteTo(w io.Writer) (n int64, err error) {
	for {
		// Write unread data from previous read.
		if len(r.unread) > 0 {
			m, err := w.Write(r.unread)
			n += int64(m)
			if err != nil {
				return n, err
			}
			r.unread = r.unread[m:]
			if len(r.unread) > 0 {
				return n, nil
			}
		}

		// The underlying Reader already returned EOF, do not read again.
		if r.eof {
			return n, io.EOF
		}

		// Read new line, including delim.
		r.unread, err = r.reader.ReadBytes('\n')

		if err == io.EOF {
			r.eof = true
		}

		// No new data, do not block.
		if len(r.unread) == 0 {
			return n, err
		}

		// Some new data, prepend prefix.
		// TODO: We could write the prefix to r.unread buffer just once
		//       and re-use it instead of prepending every time.
		r.unread = append(r.prefix, r.unread...)

		if err != nil {
			if err == io.EOF && len(r.unread) > 0 {
				// The underlying Reader already returned EOF, but we still
				// have some unread data to send, thus clear the error.
				return n, nil
			}
			return n, err
		}
	}
	panic("unreachable")
}