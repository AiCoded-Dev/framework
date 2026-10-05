package mcp

import (
	"context"

	mcpsdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"aicoded.dev/framework/cmd/aicoded/internal/explain"
	"aicoded.dev/framework/docs"
)

const (
	markdown     = "text/markdown"
	changelogURI = "aicoded://changelog"
	docsURI      = "aicoded://docs/"
	errorsURI    = "aicoded://errors/"
)

type howtoIn struct {
	Topic string `json:"topic,omitempty" jsonschema:"a page of the docs such as guides/overview, changelog or an error code such as E-DEV-001; leave it out to list the pages"`
}

type howtoOut struct {
	Topics []topic `json:"topics,omitempty"`
	Text   string  `json:"text,omitempty"`
}

type topic struct {
	Name    string `json:"name"`
	Title   string `json:"title"`
	Summary string `json:"summary,omitempty"`
}

// howto lists the topics: the guides, the tasks, the changelog and the error codes. Given a
// topic, it returns its text.
func howto(_ context.Context, in howtoIn) (howtoOut, error) {
	if in.Topic != "" {
		text, err := explain.Read(in.Topic)
		return howtoOut{Text: text}, err
	}
	ts, err := docs.Topics()
	if err != nil {
		return howtoOut{}, err
	}
	entries, err := explain.List()
	if err != nil {
		return howtoOut{}, err
	}
	var topics []topic
	for _, t := range ts {
		topics = append(topics, topic{Name: t.Path, Title: t.Title, Summary: t.Summary})
	}
	for _, e := range entries {
		topics = append(topics, topic{Name: e.Code, Title: e.Title})
	}
	return howtoOut{Topics: topics}, nil
}

// addDocs serves every guide and task as aicoded://docs/<path>, the changelog for agents as
// aicoded://changelog and every error page as aicoded://errors/<CODE>.
func addDocs(srv *mcpsdk.Server) error {
	ts, err := docs.Topics()
	if err != nil {
		return err
	}
	for _, t := range ts {
		uri := docsURI + t.Path
		if t.Path == "changelog" {
			uri = changelogURI
		}
		srv.AddResource(&mcpsdk.Resource{URI: uri, Name: t.Path, Title: t.Title, Description: t.Summary, MIMEType: markdown},
			func(context.Context, *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
				text, err := docs.Read(t.Path)
				return resource(uri, text, err)
			})
	}
	entries, err := explain.List()
	if err != nil {
		return err
	}
	for _, e := range entries {
		uri := errorsURI + e.Code
		srv.AddResource(&mcpsdk.Resource{URI: uri, Name: e.Code, Title: e.Title, MIMEType: markdown},
			func(context.Context, *mcpsdk.ReadResourceRequest) (*mcpsdk.ReadResourceResult, error) {
				text, err := explain.Page(e.Code)
				return resource(uri, text, err)
			})
	}
	return nil
}

func resource(uri, text string, err error) (*mcpsdk.ReadResourceResult, error) {
	if err != nil {
		return nil, err
	}
	return &mcpsdk.ReadResourceResult{Contents: []*mcpsdk.ResourceContents{{URI: uri, MIMEType: markdown, Text: text}}}, nil
}
