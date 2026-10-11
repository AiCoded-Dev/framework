package server

import (
	"context"
	"errors"
	"math"
	"slices"
	"strings"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/manifest"
	"aicoded.dev/framework/runnerproto/mailrules"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

// maxMessageID bounds the id of a message the app names.
const maxMessageID = 255

// mailService checks the app's mail against the mail rules and its email section, and hands
// what passes to mail.
type mailService struct {
	email *manifest.Email
	mail  Mail
}

// declared refuses every call of an app without an email section.
func (s *mailService) declared() error {
	if s.email == nil {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("email is not declared in aicoded.yaml"))
	}
	return nil
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

// errNoMessage is the answer for a message id that names no message.
func errNoMessage() error {
	return connect.NewError(connect.CodeNotFound, errors.New("no such message"))
}

// messageID reports whether id may name a message: 1 to maxMessageID visible ASCII characters.
func messageID(id string) bool {
	return id != "" && len(id) <= maxMessageID && !strings.ContainsFunc(id, func(r rune) bool { return r <= ' ' || r > '~' })
}

func (s *mailService) Send(ctx context.Context, req *connect.Request[runnerv1.SendRequest]) (*connect.Response[runnerv1.SendResponse], error) {
	if err := s.declared(); err != nil {
		return nil, err
	}
	m := req.Msg
	if err := mailrules.Check(m); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	for _, a := range slices.Concat(m.GetTo(), m.GetCc(), m.GetBcc()) {
		if !inDomains(s.email, a.GetAddress()) {
			return nil, errDomain(a.GetAddress())
		}
	}
	if r := m.GetReplyTo(); r != nil && !strings.EqualFold(r.GetAddress(), s.email.From) && !inDomains(s.email, r.GetAddress()) {
		return nil, errDomain(r.GetAddress())
	}
	id, err := s.mail.Send(ctx, s.email.From, m)
	if err != nil {
		return nil, providerError(err)
	}
	return connect.NewResponse(&runnerv1.SendResponse{Id: id}), nil
}

func (s *mailService) List(ctx context.Context, req *connect.Request[runnerv1.ListRequest]) (*connect.Response[runnerv1.ListResponse], error) {
	if err := s.declared(); err != nil {
		return nil, err
	}
	folder := req.Msg.GetFolder()
	if err := mailrules.Folder(folder); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	offset, limit := mailrules.Page(int(req.Msg.GetOffset()), int(req.Msg.GetLimit()))
	page, total, err := s.mail.List(ctx, folder, offset, limit)
	if err != nil {
		return nil, providerError(err)
	}
	if len(page) > limit {
		page = page[:limit]
	}
	return connect.NewResponse(&runnerv1.ListResponse{Messages: page, Total: int32(min(max(total, 0), math.MaxInt32))}), nil
}

func (s *mailService) Get(ctx context.Context, req *connect.Request[runnerv1.GetRequest]) (*connect.Response[runnerv1.GetResponse], error) {
	if err := s.declared(); err != nil {
		return nil, err
	}
	if !messageID(req.Msg.GetId()) {
		return nil, errNoMessage()
	}
	m, err := s.mail.Get(ctx, req.Msg.GetId())
	if err != nil {
		return nil, providerError(err)
	}
	return connect.NewResponse(m), nil
}

func (s *mailService) Delete(ctx context.Context, req *connect.Request[runnerv1.DeleteRequest]) (*connect.Response[runnerv1.DeleteResponse], error) {
	if err := s.declared(); err != nil {
		return nil, err
	}
	if !messageID(req.Msg.GetId()) {
		return nil, errNoMessage()
	}
	if err := s.mail.Delete(ctx, req.Msg.GetId()); err != nil {
		return nil, providerError(err)
	}
	return connect.NewResponse(&runnerv1.DeleteResponse{}), nil
}

func (s *mailService) Move(ctx context.Context, req *connect.Request[runnerv1.MoveRequest]) (*connect.Response[runnerv1.MoveResponse], error) {
	if err := s.declared(); err != nil {
		return nil, err
	}
	if err := mailrules.Folder(req.Msg.GetFolder()); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if !messageID(req.Msg.GetId()) {
		return nil, errNoMessage()
	}
	if err := s.mail.Move(ctx, req.Msg.GetId(), req.Msg.GetFolder()); err != nil {
		return nil, providerError(err)
	}
	return connect.NewResponse(&runnerv1.MoveResponse{}), nil
}
