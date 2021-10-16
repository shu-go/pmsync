package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/xerrors"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type listCmd struct {
	Format string `cli:"format,f" default:"{id} {subject} ({date})" help:"{id}, {subject}, {date}, {snippet}, {body}"`
}

func (c listCmd) Run(g globalCmd, args []string) error {
	config, err := getConfig(g.Credentials, g.ClientID, g.ClientSecret)
	if err != nil {
		return xerrors.Errorf("failed to get config: %v", err)
	}

	/*client*/
	_, token, err := getClient(config, g.Token, g.AuthPort)
	if err != nil {
		return xerrors.Errorf("failed to connect services: %v", err)
	}

	ctx := context.Background()
	gmailService, err := gmail.NewService(ctx, option.WithTokenSource(config.TokenSource(ctx, token)))

	q := strings.Join(args, " ")

	// check for label Notes/pomera_sync
	var pomeraSync *gmail.Label
	{
		labelsService := gmail.NewUsersLabelsService(gmailService)
		resp, err := labelsService.List(g.UserID).Do()
		if err != nil {
			return err
		}
		for _, lbl := range resp.Labels {
			if lbl.Name == g.Label {
				pomeraSync = lbl
			}
		}
		if pomeraSync == nil {
			return fmt.Errorf("Label %q not found", g.Label)
		}
	}

	// list messages
	{
		msgService := gmail.NewUsersMessagesService(gmailService)
		resp, err := msgService.List(g.UserID).LabelIds(pomeraSync.Id).Q(q).Do()
		if err != nil {
			return err
		}
		for _, msg := range resp.Messages {
			//m, err := msgService.Get(c.LoginID, msg.Id).Format("metadata").Do()
			m, err := msgService.Get(g.UserID, msg.Id).Format("full").Do()
			if err != nil {
				return err
			}

			content := c.Format

			if strings.Index(c.Format, "{id}") != -1 {
				content = strings.Replace(content, "{id}", m.Id, -1)
			}

			if strings.Index(c.Format, "{subject}") != -1 {
				content = strings.Replace(content, "{subject}", getHeader(m.Payload.Headers, "Subject"), -1)
			}

			if strings.Index(c.Format, "{headers}") != -1 {
				content = strings.Replace(content, "{headers}", fmt.Sprintf("%#v", m.Payload.Headers), -1)
			}

			if strings.Index(c.Format, "{date}") != -1 {
				content = strings.Replace(content, "{date}", getHeader(m.Payload.Headers, "Date"), -1)
			}

			if strings.Index(c.Format, "{snippet}") != -1 {
				content = strings.Replace(content, "{snippet}", m.Snippet, -1)
			}

			if strings.Index(c.Format, "{body}") != -1 {
				decoded, err := base64.URLEncoding.DecodeString(m.Payload.Body.Data)
				if err != nil {
					println("ERROR: " + err.Error())
					break //continue
				}
				content = strings.Replace(content, "{body}", string(decoded), -1)
			}

			println(content)
		}
	}

	return nil
}
