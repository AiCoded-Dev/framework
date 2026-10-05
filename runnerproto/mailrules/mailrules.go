// Package mailrules holds the rules every message must follow before a runner sends it: the
// idempotency key, plain addresses, header fields without line breaks or control characters,
// the allowed headers, sizes, mailbox folders and the pages of a mailbox list. A runner adds
// the app's own policy on top: the sender and the domains it may write to. mailer checks the
// same rules before it asks the runner.
package mailrules

import (
	"fmt"
	"mime"
	"net/mail"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"aicoded.dev/framework/internal/errs"
	"aicoded.dev/framework/runnerproto/runnerv1"
)

const (
	// MaxRecipients bounds To, Cc and Bcc together.
	MaxRecipients = 50
	// MaxAttachments bounds the attachments of one message.
	MaxAttachments = 20
	// MaxSize bounds the subject, the bodies and the attachments with their content types of one
	// message together, in bytes.
	MaxSize = 10 << 20
	// DefaultList is the number of messages a mailbox list returns when it asks for 0 or fewer.
	DefaultList = 50
	// MaxList bounds the number of messages one mailbox list returns.
	MaxList = 100

	maxKey     = 128
	maxLine    = 998
	maxName    = 100
	maxAddress = 254
	maxFile    = 255
	maxType    = 255
	maxQuote   = 64
)

// Folders are the folders of an app's mailbox.
var Folders = []string{"inbox", "archive", "trash"}

// headers are the only headers a message may set besides the ones the runner writes.
var headers = []string{"auto-submitted", "in-reply-to", "list-id", "list-unsubscribe", "list-unsubscribe-post", "precedence", "references"}

// domainName is the grammar of a domain in the email section of aicoded.yaml.
var domainName = regexp.MustCompile(`^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$`)

// Check returns the first rule msg breaks, as a coded error, or nil.
func Check(msg *runnerv1.SendRequest) error {
	if k := msg.GetIdempotencyKey(); k == "" || len(k) > maxKey || !visible(k) {
		return errs.New("E-MAIL-001", "the message needs an idempotency key of 1 to 128 visible ASCII characters",
			`set IdempotencyKey to a stable name of what the message is about, such as "invoice-42-reminder"`)
	}
	if err := displayName("the sender's name", msg.GetFromName()); err != nil {
		return err
	}
	rcpt := slices.Concat(msg.GetTo(), msg.GetCc(), msg.GetBcc())
	if len(rcpt) == 0 {
		return errAddress("the message has no recipients")
	}
	if len(rcpt) > MaxRecipients {
		return errSize(fmt.Sprintf("the message has %d recipients; at most %d are allowed", len(rcpt), MaxRecipients))
	}
	for _, a := range rcpt {
		if err := checkAddress(a); err != nil {
			return err
		}
	}
	if r := msg.GetReplyTo(); r != nil {
		if err := checkAddress(r); err != nil {
			return err
		}
	}
	if err := field("the subject", msg.GetSubject(), maxLine); err != nil {
		return err
	}
	if msg.GetText() == "" && msg.GetHtml() == "" {
		return errs.New("E-MAIL-006", "the message has neither a text nor an HTML body", "set Text, HTML or both")
	}
	seen := map[string]bool{}
	for _, h := range msg.GetHeaders() {
		name := strings.ToLower(h.GetName())
		if !visible(h.GetName()) || !slices.Contains(headers, name) || seen[name] {
			return errField(fmt.Sprintf("the header %s is not allowed, or is set twice", quote(h.GetName())))
		}
		seen[name] = true
		if err := field("the header "+h.GetName(), h.GetValue(), maxLine); err != nil {
			return err
		}
	}
	if n := len(msg.GetAttachments()); n > MaxAttachments {
		return errSize(fmt.Sprintf("the message has %d attachments; at most %d are allowed", n, MaxAttachments))
	}
	size := len(msg.GetSubject()) + len(msg.GetText()) + len(msg.GetHtml())
	for _, a := range msg.GetAttachments() {
		f, ct := a.GetFilename(), a.GetContentType()
		if !filename(f) {
			return errField(fmt.Sprintf("the attachment name %s is not allowed", quote(f)))
		}
		if !contentType(ct) {
			return errField(fmt.Sprintf("the attachment %s has no valid content type", quote(f)))
		}
		size += len(ct) + len(a.GetData())
	}
	if size > MaxSize {
		return errSize(fmt.Sprintf("the message is %d bytes; at most %d are allowed", size, MaxSize))
	}
	return nil
}

// Address checks one bare address: at most 254 bytes of visible ASCII, one @, no display name,
// and a domain that email.to_domains in aicoded.yaml could list.
func Address(a string) error {
	if len(a) > maxAddress {
		return errAddress(fmt.Sprintf("an address is longer than %d bytes", maxAddress))
	}
	if !plain(a) {
		return errAddress(fmt.Sprintf("%s is not a plain ASCII address", quote(a)))
	}
	return nil
}

// Domain returns the lower-case domain of a, or "" when Address refuses a.
func Domain(a string) string {
	if !plain(a) {
		return ""
	}
	_, d, _ := strings.Cut(a, "@")
	return strings.ToLower(d)
}

// ValidDomain reports whether d is a lower-case domain that email.to_domains in aicoded.yaml may
// list.
func ValidDomain(d string) bool { return domainName.MatchString(d) }

// Folder checks the name of a mailbox folder.
func Folder(name string) error {
	if !slices.Contains(Folders, name) {
		return errs.New("E-MAIL-007", fmt.Sprintf("the mail folder %s does not exist", quote(name)), "use mailer.Inbox, mailer.Archive or mailer.Trash")
	}
	return nil
}

// Page returns the offset and limit of a mailbox list: a negative offset is 0, a limit of 0 or
// less is DefaultList, and a limit above MaxList is MaxList.
func Page(offset, limit int) (int, int) {
	if limit <= 0 {
		limit = DefaultList
	}
	return max(offset, 0), min(limit, MaxList)
}

func plain(a string) bool {
	if a == "" || len(a) > maxAddress || !visible(a) || strings.ContainsAny(a, "<>\",;[]") {
		return false
	}
	local, domain, _ := strings.Cut(a, "@")
	if local == "" || !domainName.MatchString(strings.ToLower(domain)) {
		return false
	}
	p, err := mail.ParseAddress(a)
	return err == nil && p.Address == a && p.Name == ""
}

// checkAddress checks the address before its name, so that errors only name a valid address.
func checkAddress(a *runnerv1.Address) error {
	if err := Address(a.GetAddress()); err != nil {
		return err
	}
	return displayName("the name of "+a.GetAddress(), a.GetName())
}

// displayName checks a name shown with an address: a header field without the bidi controls,
// which could show the name in a different order.
func displayName(what, v string) error {
	if err := field(what, v, maxName); err != nil {
		return err
	}
	if strings.ContainsFunc(v, bidi) {
		return errField(what + " has a character that changes the direction of text")
	}
	return nil
}

// field refuses invalid UTF-8, control characters and line breaks in a header field and bounds
// its length.
func field(what, v string, limit int) error {
	if len(v) > limit || !utf8.ValidString(v) || strings.ContainsFunc(v, control) {
		return errField(fmt.Sprintf("%s has a line break or a control character, is not UTF-8, or is longer than %d bytes", what, limit))
	}
	return nil
}

func filename(f string) bool {
	return f != "" && f != "." && f != ".." && len(f) <= maxFile && utf8.ValidString(f) && !strings.ContainsAny(f, "/\\\t") &&
		!strings.ContainsFunc(f, func(r rune) bool { return control(r) || bidi(r) })
}

// control reports the C0 controls other than tab, DEL, and the Unicode line breaks NEL, LS and PS.
func control(r rune) bool {
	return r < 0x20 && r != '\t' || r == 0x7f || r == 0x85 || r == 0x2028 || r == 0x2029
}

// bidi reports the Unicode embeddings, overrides and isolates, which change the order text is shown in.
func bidi(r rune) bool {
	return r >= 0x202a && r <= 0x202e || r >= 0x2066 && r <= 0x2069
}

// contentType accepts a media type of at most 255 bytes of visible ASCII, space and tab.
func contentType(ct string) bool {
	if len(ct) > maxType || strings.ContainsFunc(ct, func(r rune) bool { return r < 0x20 && r != '\t' || r > 0x7e }) {
		return false
	}
	_, _, err := mime.ParseMediaType(ct)
	return err == nil
}

func visible(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x21 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// quote quotes s for an error message, cut to 64 bytes.
func quote(s string) string {
	if len(s) > maxQuote {
		return strconv.Quote(s[:maxQuote]) + "..."
	}
	return strconv.Quote(s)
}

func errAddress(msg string) error {
	return errs.New("E-MAIL-002", msg, "write plain addresses such as name@company.example and put the person's name in Name")
}

func errField(msg string) error {
	return errs.New("E-MAIL-004", msg, "remove line breaks and control characters, and set only In-Reply-To, References, List-Id, List-Unsubscribe, List-Unsubscribe-Post, Auto-Submitted or Precedence")
}

func errSize(msg string) error {
	return errs.New("E-MAIL-005", msg, "send to fewer people at once, or save large files in a file store and link to them")
}
