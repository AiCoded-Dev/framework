package mailer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"connectrpc.com/connect"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/internal/runner"
	"aicoded.dev/framework/runnerproto/mailrules"
	"aicoded.dev/framework/runnerproto/runnerv1"
	"aicoded.dev/framework/runnerproto/runnerv1/runnerv1connect"
	"aicoded.dev/framework/telemetry"
)

// Folders of the app's mailbox.
const (
	Inbox   = "inbox"
	Archive = "archive"
	Trash   = "trash"
)

// ErrNotFound is returned for a message id the mailbox does not hold.
var ErrNotFound = errors.New("mailer: no such message")

// Address is one mailbox: a plain address and the name shown with it.
type Address struct{ Name, Address string }

// Attachment is a file sent with or received in a message.
type Attachment struct {
	Filename, ContentType string
	Data                  []byte
}

// Header is one extra header field.
type Header struct{ Name, Value string }

// Message is one outbound email. The sender is always the app's address; FromName sets the
// name shown with it. IdempotencyKey is required: sending the same key again returns the first
// message's id and sends nothing, so a retried request never mails twice.
type Message struct {
	IdempotencyKey string
	FromName       string
	To, Cc, Bcc    []Address
	ReplyTo        *Address
	Subject        string
	Text, HTML     string
	Attachments    []Attachment
	// Headers may set only In-Reply-To, References, List-Id, List-Unsubscribe,
	// List-Unsubscribe-Post, Auto-Submitted and Precedence.
	Headers []Header
}

// Summary describes a message in the mailbox.
type Summary struct {
	ID      string
	From    Address
	To      []Address
	Subject string
	Date    time.Time
	Size    int64
	Folder  string
}

// Mail is a whole message from the mailbox.
type Mail struct {
	Summary
	Cc          []Address
	ReplyTo     *Address
	Text, HTML  string
	Attachments []Attachment
	Headers     []Header
}

// Send checks msg against the mail rules and hands it to the runner, and returns its id.
func Send(ctx context.Context, msg Message) (string, error) {
	req := sendRequest(msg)
	if err := mailrules.Check(req); err != nil {
		return "", err
	}
	var id string
	err := call(ctx, "mailer.send", func(ctx context.Context, c runnerv1connect.MailServiceClient) error {
		resp, err := c.Send(ctx, connect.NewRequest(req))
		if err != nil {
			return err
		}
		id = resp.Msg.GetId()
		return nil
	})
	return id, err
}

// List returns a page of folder, newest first, and the number of messages in the folder. The
// page skips offset messages and holds up to limit, and never more than 100. A negative offset
// counts as 0, and a limit of 0 or less as 50.
func List(ctx context.Context, folder string, offset, limit int) ([]Summary, int, error) {
	if err := mailrules.Folder(folder); err != nil {
		return nil, 0, err
	}
	offset, limit = mailrules.Page(offset, limit)
	var resp *connect.Response[runnerv1.ListResponse]
	err := call(ctx, "mailer.list", func(ctx context.Context, c runnerv1connect.MailServiceClient) (err error) {
		resp, err = c.List(ctx, connect.NewRequest(&runnerv1.ListRequest{Folder: folder, Offset: int32Of(offset), Limit: int32Of(limit)}))
		return err
	})
	if err != nil {
		return nil, 0, err
	}
	out := make([]Summary, len(resp.Msg.GetMessages()))
	for i, m := range resp.Msg.GetMessages() {
		out[i] = summary(m)
	}
	return out, int(resp.Msg.GetTotal()), nil
}

// Get returns the message id, or ErrNotFound.
func Get(ctx context.Context, id string) (*Mail, error) {
	var resp *connect.Response[runnerv1.GetResponse]
	err := call(ctx, "mailer.get", func(ctx context.Context, c runnerv1connect.MailServiceClient) (err error) {
		resp, err = c.Get(ctx, connect.NewRequest(&runnerv1.GetRequest{Id: id}))
		return err
	})
	if err != nil {
		return nil, err
	}
	m := resp.Msg
	out := &Mail{Summary: summary(m.GetSummary()), Cc: addresses(m.GetCc()), Text: m.GetText(), HTML: m.GetHtml()}
	if r := m.GetReplyTo(); r != nil {
		out.ReplyTo = &Address{Name: r.GetName(), Address: r.GetAddress()}
	}
	for _, a := range m.GetAttachments() {
		out.Attachments = append(out.Attachments, Attachment{Filename: a.GetFilename(), ContentType: a.GetContentType(), Data: a.GetData()})
	}
	for _, h := range m.GetHeaders() {
		out.Headers = append(out.Headers, Header{Name: h.GetName(), Value: h.GetValue()})
	}
	return out, nil
}

// Delete removes the message id from the mailbox, or returns ErrNotFound.
func Delete(ctx context.Context, id string) error {
	return call(ctx, "mailer.delete", func(ctx context.Context, c runnerv1connect.MailServiceClient) error {
		_, err := c.Delete(ctx, connect.NewRequest(&runnerv1.DeleteRequest{Id: id}))
		return err
	})
}

// Move moves the message id to folder, or returns ErrNotFound.
func Move(ctx context.Context, id, folder string) error {
	if err := mailrules.Folder(folder); err != nil {
		return err
	}
	return call(ctx, "mailer.move", func(ctx context.Context, c runnerv1connect.MailServiceClient) error {
		_, err := c.Move(ctx, connect.NewRequest(&runnerv1.MoveRequest{Id: id, Folder: folder}))
		return err
	})
}

// call runs f in a span named name. Outside aicoded dev, a failed call's span keeps only its
// status code, since the runner's message may name a recipient's domain.
func call(ctx context.Context, name string, f func(context.Context, runnerv1connect.MailServiceClient) error) error {
	s := runner.From(ctx)
	if s == nil || s.Mail == nil {
		return runner.ErrNoRunner
	}
	ctx, span := telemetry.Start(ctx, name)
	defer span.End()
	if err := f(ctx, s.Mail); err != nil {
		span.RecordError(err)
		return mailErr(err)
	}
	return nil
}

func mailErr(err error) error {
	switch connect.CodeOf(err) {
	case connect.CodeFailedPrecondition:
		return errs.New("E-MAN-012", "the app sends or reads mail, but aicoded.yaml has no email section",
			"add email: with from and to_domains to aicoded.yaml")
	case connect.CodeNotFound:
		return ErrNotFound
	case connect.CodeInvalidArgument:
		var ce *connect.Error
		if errors.As(err, &ce) {
			if e, ok := errs.Parse(ce.Message()); ok {
				return e
			}
		}
	}
	return fmt.Errorf("mailer: %w", err)
}

// int32Of bounds n to the protocol's int32 fields.
func int32Of(n int) int32 {
	return int32(min(max(n, math.MinInt32), math.MaxInt32))
}

func sendRequest(m Message) *runnerv1.SendRequest {
	req := &runnerv1.SendRequest{
		IdempotencyKey: m.IdempotencyKey, FromName: m.FromName,
		To: protoAddresses(m.To), Cc: protoAddresses(m.Cc), Bcc: protoAddresses(m.Bcc),
		Subject: m.Subject, Text: m.Text, Html: m.HTML,
	}
	if m.ReplyTo != nil {
		req.ReplyTo = &runnerv1.Address{Name: m.ReplyTo.Name, Address: m.ReplyTo.Address}
	}
	for _, a := range m.Attachments {
		req.Attachments = append(req.Attachments, &runnerv1.Attachment{Filename: a.Filename, ContentType: a.ContentType, Data: a.Data})
	}
	for _, h := range m.Headers {
		req.Headers = append(req.Headers, &runnerv1.Header{Name: h.Name, Value: h.Value})
	}
	return req
}

func protoAddresses(as []Address) []*runnerv1.Address {
	out := make([]*runnerv1.Address, len(as))
	for i, a := range as {
		out[i] = &runnerv1.Address{Name: a.Name, Address: a.Address}
	}
	return out
}

func addresses(as []*runnerv1.Address) []Address {
	out := make([]Address, len(as))
	for i, a := range as {
		out[i] = Address{Name: a.GetName(), Address: a.GetAddress()}
	}
	return out
}

func summary(m *runnerv1.MessageSummary) Summary {
	return Summary{
		ID: m.GetId(), From: Address{Name: m.GetFrom().GetName(), Address: m.GetFrom().GetAddress()},
		To: addresses(m.GetTo()), Subject: m.GetSubject(), Date: time.Unix(m.GetDateUnix(), 0),
		Size: m.GetSize(), Folder: m.GetFolder(),
	}
}
