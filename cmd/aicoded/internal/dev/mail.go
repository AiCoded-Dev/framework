package dev

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

// mailService is the app's mail catcher. The runner server hands it mail that passed the mail
// rules and the permission list, and it keeps that mail instead of sending it: the last 1000
// messages, with the secret values hidden. It also serves the app's mailbox from memory.
type mailService struct {
	hide    func() *redactor
	changes *changes // told of every message sent, received, moved or deleted; may be nil
	mu      sync.Mutex
	email   *manifest.Email
	sent    *Ring[*runnerv1.GetResponse]
	count   int
	keys    map[string]string
	box     []*runnerv1.GetResponse
}

// newMailService returns a catcher for the mail rules email that hides the secret values of
// hide, which may be nil.
func newMailService(email *manifest.Email, hide func() *redactor) *mailService {
	if hide == nil {
		hide = func() *redactor { return nil }
	}
	return &mailService{hide: hide, email: email, sent: NewRing[*runnerv1.GetResponse](mailCount), keys: map[string]string{}}
}

// setEmail makes email the mail rules of the app from now on.
func (s *mailService) setEmail(email *manifest.Email) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.email = email
}

// declared reports whether the app declares email.
func (s *mailService) declared() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.email != nil
}

// Send keeps msg as sent from the address from, once for each idempotency key.
func (s *mailService) Send(_ context.Context, from string, m *runnerv1.SendRequest) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.keys[m.GetIdempotencyKey()]; ok {
		return id, nil
	}
	s.count++
	id := fmt.Sprintf("sent-%d", s.count)
	s.keys[m.GetIdempotencyKey()] = id
	size := len(m.GetSubject()) + len(m.GetText()) + len(m.GetHtml())
	for _, a := range m.GetAttachments() {
		size += len(a.GetData())
	}
	sent := proto.Clone(&runnerv1.GetResponse{
		Summary: &runnerv1.MessageSummary{
			Id:       id,
			From:     &runnerv1.Address{Name: m.GetFromName(), Address: from},
			To:       m.GetTo(),
			Subject:  m.GetSubject(),
			DateUnix: time.Now().Unix(),
			Size:     int64(size),
		},
		Cc: m.GetCc(), Bcc: m.GetBcc(), ReplyTo: m.GetReplyTo(),
		Text: m.GetText(), Html: m.GetHtml(), Attachments: m.GetAttachments(), Headers: m.GetHeaders(),
	}).(*runnerv1.GetResponse)
	redactMail(sent, s.hide())
	s.sent.Add(sent)
	s.changes.signal()
	return id, nil
}

// List returns a page of the messages in folder, newest first, and how many it holds.
func (s *mailService) List(_ context.Context, folder string, offset, limit int) ([]*runnerv1.MessageSummary, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var in []*runnerv1.MessageSummary
	for i := len(s.box) - 1; i >= 0; i-- {
		if sum := s.box[i].GetSummary(); sum.GetFolder() == folder {
			in = append(in, proto.Clone(sum).(*runnerv1.MessageSummary))
		}
	}
	start := min(offset, len(in))
	return in[start:min(start+limit, len(in))], len(in), nil
}

// find returns the index of message id in the mailbox; the caller holds s.mu.
func (s *mailService) find(id string) (int, error) {
	i := slices.IndexFunc(s.box, func(m *runnerv1.GetResponse) bool { return m.GetSummary().GetId() == id })
	if i < 0 {
		return 0, connect.NewError(connect.CodeNotFound, errors.New("no such message"))
	}
	return i, nil
}

func (s *mailService) Get(_ context.Context, id string) (*runnerv1.GetResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.find(id)
	if err != nil {
		return nil, err
	}
	return proto.Clone(s.box[i]).(*runnerv1.GetResponse), nil
}

func (s *mailService) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.find(id)
	if err != nil {
		return err
	}
	s.box = slices.Delete(s.box, i, i+1)
	s.changes.signal()
	return nil
}

func (s *mailService) Move(_ context.Context, id, folder string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.find(id)
	if err != nil {
		return err
	}
	s.box[i].GetSummary().Folder = folder
	s.changes.signal()
	return nil
}

// deliver puts a copy of msg, with the secret values hidden, into the inbox as if it had
// arrived, and returns its id.
func (s *mailService) deliver(msg *runnerv1.GetResponse) string {
	m := proto.Clone(msg).(*runnerv1.GetResponse)
	if m.Summary == nil {
		m.Summary = &runnerv1.MessageSummary{}
	}
	m.Summary.Id, m.Summary.Folder = rand.Text(), "inbox"
	if m.Summary.DateUnix == 0 {
		m.Summary.DateUnix = time.Now().Unix()
	}
	redactMail(m, s.hide())
	s.mu.Lock()
	defer s.mu.Unlock()
	s.box = append(s.box, m)
	s.changes.signal()
	return m.Summary.Id
}

// sentMail returns copies of the mail the app sent, oldest first. The local runner never sends mail.
func (s *mailService) sentMail() []*runnerv1.GetResponse {
	sent := s.sent.All()
	for i, m := range sent {
		sent[i] = proto.Clone(m).(*runnerv1.GetResponse)
	}
	return sent
}

// summaries returns the sent mail, then the mailbox, each newest first.
func (s *mailService) summaries() []devapi.MailSummary {
	sent := s.sent.All()
	s.mu.Lock()
	box := slices.Clone(s.box)
	for i, m := range box {
		box[i] = proto.Clone(m).(*runnerv1.GetResponse)
	}
	s.mu.Unlock()
	out := []devapi.MailSummary{}
	for _, m := range slices.Backward(sent) {
		out = append(out, toMail(m, "sent").MailSummary)
	}
	for _, m := range slices.Backward(box) {
		out = append(out, toMail(m, m.GetSummary().GetFolder()).MailSummary)
	}
	return out
}

// get returns the message id, sent or in the mailbox.
func (s *mailService) get(id string) (devapi.Mail, bool) {
	for _, m := range s.sent.All() {
		if m.GetSummary().GetId() == id {
			return toMail(m, "sent"), true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if i, err := s.find(id); err == nil {
		return toMail(s.box[i], s.box[i].GetSummary().GetFolder()), true
	}
	return devapi.Mail{}, false
}

// redactMail hides the secret values of hide in the text of m.
func redactMail(m *runnerv1.GetResponse, hide *redactor) {
	if hide == nil {
		return
	}
	sum := m.GetSummary()
	sum.Subject = hide.String(sum.GetSubject())
	m.Text, m.Html = hide.String(m.GetText()), hide.String(m.GetHtml())
	for _, a := range slices.Concat([]*runnerv1.Address{sum.GetFrom(), m.GetReplyTo()}, sum.GetTo(), m.GetCc(), m.GetBcc()) {
		if a != nil {
			a.Name = hide.String(a.GetName())
		}
	}
	for _, h := range m.GetHeaders() {
		h.Value = hide.String(h.GetValue())
	}
	for _, a := range m.GetAttachments() {
		a.Filename = hide.String(a.GetFilename())
		a.Data = []byte(hide.String(string(a.GetData())))
	}
}

// toMail turns m, a message in folder, into what aicoded dev shows of it.
func toMail(m *runnerv1.GetResponse, folder string) devapi.Mail {
	sum := m.GetSummary()
	out := devapi.Mail{
		MailSummary: devapi.MailSummary{
			ID: sum.GetId(), Folder: folder, From: sum.GetFrom().GetAddress(), To: addresses(sum.GetTo()),
			Subject: sum.GetSubject(), Time: time.Unix(sum.GetDateUnix(), 0),
		},
		Cc: addresses(m.GetCc()), Bcc: addresses(m.GetBcc()), ReplyTo: m.GetReplyTo().GetAddress(),
		Text: m.GetText(), HTML: m.GetHtml(),
	}
	for _, a := range m.GetAttachments() {
		out.Attachments = append(out.Attachments, devapi.Attachment{Name: a.GetFilename(), Type: a.GetContentType(), Size: len(a.GetData())})
	}
	return out
}

func addresses(as []*runnerv1.Address) []string {
	out := []string{}
	for _, a := range as {
		out = append(out, a.GetAddress())
	}
	return out
}
