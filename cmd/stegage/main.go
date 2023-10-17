// Copyright (c) 2021 The stegage developers

package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/revelaction/stegage"
	"github.com/urfave/cli/v2"
	"golang.org/x/term"
)

const (
	BASH_COMPLETE = `#! /bin/bash

: ${PROG:=$(basename ${BASH_SOURCE})}

_cli_bash_autocomplete() {
  if [[ "${COMP_WORDS[0]}" != "source" ]]; then
    local cur opts base
    COMPREPLY=()
    cur="${COMP_WORDS[COMP_CWORD]}"
    if [[ "$cur" == "-"* ]]; then
      opts=$( ${COMP_WORDS[@]:0:$COMP_CWORD} ${cur} --generate-bash-completion )
    else
      opts=$( ${COMP_WORDS[@]:0:$COMP_CWORD} --generate-bash-completion )
    fi
    COMPREPLY=( $(compgen -W "${opts}" -- ${cur}) )
    return 0
  fi
}

complete -o bashdefault -o default -o nospace -F _cli_bash_autocomplete $PROG
unset PROG`
)

var (
	BuildCommit string
	BuildTag    string
)

func main() {

	app := &cli.App{}

	app.Name = "stegage"
	app.Version = BuildTag
	cli.VersionPrinter = func(c *cli.Context) {
		fmt.Printf("%s version %s, commit %s\n", app.Name, c.App.Version, BuildCommit)
	}

	app.EnableBashCompletion = true
	app.UseShortOptionHandling = true
	app.Usage = "encrypt and conceal (steganography) a file"

	// encode command
	command := &cli.Command{
		Name:    "encode",
		Aliases: []string{"e"},
		Action:  encodeAction,

		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "inside",
				Aliases:  []string{"i"},
				Usage:    "image file. Supported formats: jpeg, png",
				Required: true,
			},
		},
		Usage: "Encrypt a file with age and additionally conceal with steganography",
	}

	app.Commands = append(app.Commands, command)

	// decode command
	command = &cli.Command{
		Name:    "decode",
		Aliases: []string{"d"},
		Action:  decodeAction,
		Usage:   "Decrypt an age encrypted file inside of an image file",
	}

	app.Commands = append(app.Commands, command)

	//bash complete command
	command = &cli.Command{
		Name:   "bash",
		Usage:  "Dump bash complete script",
		Action: bashAction,
	}
	app.Commands = append(app.Commands, command)

	if err := app.Run(os.Args); err != nil {
		fatal(err)
	}
}

func encodeAction(ctx *cli.Context) error {

	if ctx.Args().Len() > 1 {
		cli.ShowAppHelp(ctx)
		return fmt.Errorf("Max number of arguments is 1")
	}

	var in io.Reader = os.Stdin

	// data file
	if ctx.Args().Len() == 1 {
		fileName := ctx.Args().First()
		f, err := os.Open(fileName)
		if err != nil {
			return fmt.Errorf("could not open file: %w", err)
		}

		defer f.Close()

		in = f
	}

	// password
	pass, err := passphrasePromptForEncryption()
	if err != nil {
		return err
	}

	passReader := bytes.NewReader([]byte(pass))

	// image file, this is a required flag
	hostImage := ctx.String("inside")

	imageIn, err := os.Open(hostImage)
	if err != nil {
		return fmt.Errorf("could not open file: %w", err)
	}

	defer imageIn.Close()

	// Per default we go to Stdout.
	var out io.Writer = os.Stdout

	if err := stegage.Encode(passReader, in, imageIn, out); err != nil {
		return err
	}

	return nil
}

func decodeAction(ctx *cli.Context) error {

	if ctx.Args().Len() > 1 {
		cli.ShowAppHelp(ctx)
		return fmt.Errorf("Max number of arguments is 1")
	}

	var in io.Reader = os.Stdin

	// image file
	if ctx.Args().Len() == 1 {
		fileName := ctx.Args().First()
		f, err := os.Open(fileName)
		if err != nil {
			return fmt.Errorf("could not open file: %w", err)
		}

		defer f.Close()
		in = f
	}

	pass, err := readPassphrase("Enter passphrase:")
	if err != nil {
		return err
	}

	passReader := bytes.NewReader([]byte(pass))

	var out io.Writer = os.Stdout

	if err := stegage.Decode(passReader, in, out); err != nil {
		return err
	}

	return nil
}

func bashAction(ctx *cli.Context) error {

	fmt.Println(BASH_COMPLETE)
	return nil
}

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "stegage: %v\n", err)
	os.Exit(1)
}

// from https://raw.githubusercontent.com/FiloSottile/age/main/cmd/age/age.go
func passphrasePromptForEncryption() (string, error) {
	pass, err := readPassphrase("Enter passphrase:")
	if err != nil {
		return "", fmt.Errorf("could not read passphrase: %w", err)
	}
	p := string(pass)
	if p == "" {
		return "", fmt.Errorf("empty passphrase not allowed")

	} else {
		confirm, err := readPassphrase("Confirm passphrase:")
		if err != nil {
			return "", fmt.Errorf("could not read passphrase: %w", err)
		}
		if string(confirm) != p {
			return "", fmt.Errorf("passphrases didn't match")
		}
	}
	return p, nil
}

// from https://raw.githubusercontent.com/FiloSottile/age/main/cmd/age/encrypted_keys.go
//
// readPassphrase reads a passphrase from the terminal. It does not read from a
// non-terminal stdin, so it does not check stdinInUse.
func readPassphrase(prompt string) ([]byte, error) {
	var in, out *os.File
	if runtime.GOOS == "windows" {
		var err error
		in, err = os.OpenFile("CONIN$", os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		defer in.Close()
		out, err = os.OpenFile("CONOUT$", os.O_WRONLY, 0)
		if err != nil {
			return nil, err
		}
		defer out.Close()
	} else if _, err := os.Stat("/dev/tty"); err == nil {
		tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
		if err != nil {
			return nil, err
		}
		defer tty.Close()
		in, out = tty, tty
	} else {
		if !term.IsTerminal(int(os.Stdin.Fd())) {
			return nil, fmt.Errorf("standard input is not a terminal, and /dev/tty is not available: %v", err)
		}
		in, out = os.Stdin, os.Stderr
	}
	fmt.Fprintf(out, "%s ", prompt)
	// Use CRLF to work around an apparent bug in WSL2's handling of CONOUT$.
	// Only when running a Windows binary from WSL2, the cursor would not go
	// back to the start of the line with a simple LF. Honestly, it's impressive
	// CONIN$ and CONOUT$ even work at all inside WSL2.
	defer fmt.Fprintf(out, "\r\n")
	return term.ReadPassword(int(in.Fd()))
}
