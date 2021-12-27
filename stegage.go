// Copyright (c) 2021 The stegage developers

// Package stegage implements file encryption according to the age-encryption.org/v1
// specification, and also steganography of the encrypted file.
package stegage

import (
	"bufio"
	"bytes"
	"fmt"
	imagePkg "image"
	"io"

	// Supported formats
	_ "image/jpeg"
	_ "image/png"

	"filippo.io/age"
	"github.com/auyer/steganography"
)

// AGE_HEADER_FIRST_14 contains the first 14 bytes of a valid encrypted age
// file
const AGE_HEADER_FIRST_14 = "age-encryption"

// Encode encrypts data to the age password recipient and after that encodes
// with LSB steganography the encrypted payload in a copy of image.
//
// The resulting image format is png.
func Encode(password, data, image io.Reader, out io.Writer) error {

	passBuf := new(bytes.Buffer)
	passBuf.ReadFrom(password)
	pass := passBuf.String()

	encOut := new(bytes.Buffer)

	r, err := age.NewScryptRecipient(pass)
	if err != nil {
		return fmt.Errorf("age: could no initialize password: %v", err)
	}

	w, err := age.Encrypt(encOut, r)

	if err != nil {
		return fmt.Errorf("age: Could not encrypt file: %v", err)
	}
	if _, err := io.Copy(w, data); err != nil {
		return fmt.Errorf("stegage: Could not copy file: %v", err)
	}

	if err := w.Close(); err != nil {
		return fmt.Errorf("age: Could not close file: %v", err)
	}

	encrypted := encOut.Bytes()
	// Double check
	if !isPayloadAge(encrypted) {
		return fmt.Errorf("stegage: No encrypted payload")
	}

	img, _, err := imagePkg.Decode(image)
	if err != nil {
		return fmt.Errorf("image: could no decode image: %v", err)
	}

	imageOut := new(bytes.Buffer)

	err = steganography.Encode(imageOut, img, encrypted)
	if err != nil {
		return fmt.Errorf("steganography: could no encode image: %v", err)
	}

	bufFile := bufio.NewWriter(out)
	if _, err := io.Copy(bufFile, imageOut); err != nil {
		return fmt.Errorf("stegage: Could not copy file: %v", err)
	}

	bufFile.Flush()

	return nil
}

// Decode extract a encrypted payload of an png image and decrypts the payload
// with the password.
func Decode(password, image io.Reader, out io.Writer) error {

	r := bufio.NewReader(image)
	img, _, err := imagePkg.Decode(r)
	if err != nil {
		return fmt.Errorf("image: could no decode image: %v", err)
	}

	sizeOfMessage := steganography.GetMessageSizeFromImage(img)
	msg := steganography.Decode(sizeOfMessage, img)

	if !isPayloadAge(msg) {
		return fmt.Errorf("stegage: found no encrypted age file inside image")
	}

	b := bytes.NewBuffer(msg)

	passBuf := new(bytes.Buffer)
	passBuf.ReadFrom(password)
	pass := passBuf.String()

	i, err := age.NewScryptIdentity(pass)
	if err != nil {
		return fmt.Errorf("age: could no initialize password: %v", err)
	}

	decOut, err := age.Decrypt(b, i)
	if err != nil {
		return fmt.Errorf("age: could no decrypt data: %v", err)
	}

	outBuf := bufio.NewWriter(out)

	if _, err := io.Copy(outBuf, decOut); err != nil {
		return fmt.Errorf("stegage: Could not copy file: %v", err)
	}

	outBuf.Flush()

	return nil
}

func isPayloadAge(payload []byte) bool {
	if string(payload[:14]) != AGE_HEADER_FIRST_14 {
		return false
	}

	return true
}
