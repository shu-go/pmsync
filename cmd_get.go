package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type getCmd struct {
	OutputTarget string `cli:"output,o" default:"stdout" help:"output destination {stdout,file}"`
	OutputFormat string `cli:"format,fo" default:"{subject}.txt" help:"file name format where --output=file ({subect}, {id})"`
	OutputDest   string `cli:"dest,d" default:"./pomera_sync" help:"output directory where --output=file"`
}

func (c getCmd) Run(g globalCmd, args []string) error {
	if c.OutputTarget == "file" {
		if _, err := os.Stat(c.OutputDest); err != nil {
			err = os.MkdirAll(c.OutputDest, os.ModePerm)
			if err != nil {
				return fmt.Errorf("mkdir %v: %v", c.OutputDest, err)
			}
		}
	}

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
	if err != nil {
		return xerrors.Errorf("failed to instantiate a gmail service: %v", err)
	}

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

			var content string
			decoded, err := base64.URLEncoding.DecodeString(m.Payload.Body.Data)
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				break //continue
			}
			content = string(decoded)

			if c.OutputTarget == "file" {
				fmt.Fprintf(os.Stderr, "getting: %v\n", getHeader(m.Payload.Headers, "Subject"))

				name := c.OutputFormat
				if strings.Contains(c.OutputFormat, "{subject}") {
					name = strings.Replace(name, "{subject}", getHeader(m.Payload.Headers, "Subject"), -1)
				}

				if c.OutputDest != "" {
					name = filepath.Join(c.OutputDest, name)
				}

				file, err := os.Create(name)
				if err != nil {
					return fmt.Errorf("create %v: %v", name, err)
				}
				_, err = file.WriteString(content)
				if err != nil {
					return fmt.Errorf("create %v: %v", name, err)
				}
				file.Close()
			} else {
				fmt.Println(content)
			}
		}
	}

	return nil
}
