package main

import (
	"bytes"
	"io"
	"mime"
	"mime/quotedprintable"
	"net/mail"
	"strings"
	"testing"

	"github.com/emersion/go-imap/v2"
)

func TestBuildReply(t *testing.T) {
	envelope := &imap.Envelope{
		Subject:   "Тема",
		MessageID: "original@example.com",
		From:      []imap.Address{{Mailbox: "sender", Host: "example.com"}},
		ReplyTo:   []imap.Address{{Mailbox: "reply", Host: "example.com"}},
	}
	raw, err := buildReply("owner@example.com", envelope, "Здравейте!")
	if err != nil {
		t.Fatal(err)
	}
	msg, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Get("To") != "<reply@example.com>" || msg.Header.Get("In-Reply-To") != "<original@example.com>" {
		t.Fatal("unexpected recipient or threading header", msg.Header)
	}
	subject, err := new(mime.WordDecoder).DecodeHeader(msg.Header.Get("Subject"))
	if err != nil || subject != "Re: Тема" {
		t.Fatalf("unexpected subject %q: %v", subject, err)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(msg.Body))
	if err != nil || string(body) != "Здравейте!" {
		t.Fatalf("unexpected body %q: %v", body, err)
	}
	envelope.Subject = "Subject\r\nBcc: outsider@example.com"
	if _, err := buildReply("owner@example.com", envelope, "body"); err == nil || !strings.Contains(err.Error(), "subject") {
		t.Fatal("accepted injected header")
	}
}
