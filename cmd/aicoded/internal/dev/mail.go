package dev

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"sync"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"

	"aicoded.dev/framework/cmd/aicoded/internal/devapi"
	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/mailrules"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

// mailService is the app's mail catcher. It checks outbound mail against the mail rules and
// the permission list, and keeps it instead of sending it: the last 1000 messages, with the
// secret values hidden. It also serves the app's mailbox from memory.
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

// rules returns the mail rules, or an error when the app declares none.
func (s *mailService) rules() (*manifest.Email, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.email == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("email is not declared in aicoded.yaml"))
	}
	return s.email, nil
}

// inDomains reports whether address is in email.to_domains.
func inDomains(email *manifest.Email, address string) bool {
	return slices.Contains(email.ToDomains, mailrules.Domain(address))
}

// errDomain names only the domain of address, so that the error never repeats a person's address.
func errDomain(address string) error {
	return connect.NewError(connect.CodeInvalidArgument, errs.New("E-MAIL-003",
		"the domain "+quote(mailrules.Domain(address))+" is not in email.to_domains in aicoded.yaml",
		"send only to the domains in email.to_domains; to mail another domain, add it to email.to_domains"))
}

func (s *mailService) Send(_ context.Context, req *connect.Request[runnerv1.SendRequest]) (*connect.Response[runnerv1.SendResponse], error) {
	email, err := s.rules()
	if err != nil {
		return nil, err
	}
	m := req.Msg
	if err := mailrules.Check(m); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	for _, a := range slices.Concat(m.GetTo(), m.GetCc(), m.GetBcc()) {
		if !inDomains(email, a.GetAddress()) {
			return nil, errDomain(a.GetAddress())
		}
	}
	if r := m.GetReplyTo(); r != nil && !strings.EqualFold(r.GetAddress(), email.From) && !inDomains(email, r.GetAddress()) {
		return nil, errDomain(r.GetAddress())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if id, ok := s.keys[m.GetIdempotencyKey()]; ok {
		return connect.NewResponse(&runnerv1.SendResponse{Id: id}), nil
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
			From:     &runnerv1.Address{Name: m.GetFromName(), Address: email.From},
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
	return connect.NewResponse(&runnerv1.SendResponse{Id: id}), nil
}

func (s *mailService) List(_ context.Context, req *connect.Request[runnerv1.ListRequest]) (*connect.Response[runnerv1.ListResponse], error) {
	if _, err := s.rules(); err != nil {
		return nil, err
	}
	folder := req.Msg.GetFolder()
	if err := mailrules.Folder(folder); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	offset, limit := mailrules.Page(int(req.Msg.GetOffset()), int(req.Msg.GetLimit()))
	s.mu.Lock()
	defer s.mu.Unlock()
	var in []*runnerv1.MessageSummary
	for i := len(s.box) - 1; i >= 0; i-- {
		if sum := s.box[i].GetSummary(); sum.GetFolder() == folder {
			in = append(in, proto.Clone(sum).(*runnerv1.MessageSummary))
		}
	}
	total := len(in)
	if total > math.MaxInt32 {
		total = math.MaxInt32
	}
	start := min(offset, len(in))
	page := in[start:min(start+limit, len(in))]
	return connect.NewResponse(&runnerv1.ListResponse{Messages: page, Total: int32(total)}), nil
}

// find returns the index of message id in the mailbox; the caller holds s.mu.
func (s *mailService) find(id string) (int, error) {
	i := slices.IndexFunc(s.box, func(m *runnerv1.GetResponse) bool { return m.GetSummary().GetId() == id })
	if i < 0 {
		return 0, connect.NewError(connect.CodeNotFound, errors.New("no such message"))
	}
	return i, nil
}

func (s *mailService) Get(_ context.Context, req *connect.Request[runnerv1.GetRequest]) (*connect.Response[runnerv1.GetResponse], error) {
	if _, err := s.rules(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.find(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(proto.Clone(s.box[i]).(*runnerv1.GetResponse)), nil
}

func (s *mailService) Delete(_ context.Context, req *connect.Request[runnerv1.DeleteRequest]) (*connect.Response[runnerv1.DeleteResponse], error) {
	if _, err := s.rules(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.find(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	s.box = slices.Delete(s.box, i, i+1)
	s.changes.signal()
	return connect.NewResponse(&runnerv1.DeleteResponse{}), nil
}

func (s *mailService) Move(_ context.Context, req *connect.Request[runnerv1.MoveRequest]) (*connect.Response[runnerv1.MoveResponse], error) {
	if _, err := s.rules(); err != nil {
		return nil, err
	}
	if err := mailrules.Folder(req.Msg.GetFolder()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	i, err := s.find(req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	s.box[i].GetSummary().Folder = req.Msg.GetFolder()
	s.changes.signal()
	return connect.NewResponse(&runnerv1.MoveResponse{}), nil
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
