package printer

import (
	"fmt"
	"io"
	"os"
)

type Printer interface {
	Error(string)
	Errorf(string, ...any)
	Info(string)
	Infof(string, ...any)
	Fatal(int, string)
	Fatalf(int, string, ...any)
}

type printer struct {
	out    io.Writer
	errOut io.Writer
}

func New(out io.Writer, errOut io.Writer) Printer {
	return &printer{out: out, errOut: errOut}
}

func NewConsole() Printer {
	return &printer{out: os.Stdout, errOut: os.Stderr}
}

func (p printer) Error(s string) {
	fmt.Fprint(p.errOut, s)
}

func (p printer) Errorf(s string, a ...any) {
	fmt.Fprintf(p.errOut, s, a...)
}

func (p printer) Info(s string) {
	fmt.Fprint(p.out, s)
}

func (p printer) Infof(s string, a ...any) {
	fmt.Fprintf(p.out, s, a...)
}

func (p printer) Fatal(code int, s string) {
	fmt.Fprint(p.errOut, s)
	os.Exit(code)
}

func (p printer) Fatalf(code int, s string, a ...any) {
	fmt.Fprintf(p.errOut, s, a...)
	os.Exit(code)
}
