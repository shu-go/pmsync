package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	zglob "github.com/mattn/go-zglob"
	"golang.org/x/xerrors"
	gmail "google.golang.org/api/gmail/v1"
	"google.golang.org/api/option"
)

type putCmd struct {
	InputSrc string `cli:"src,s" default:"./pomera_sync" help:"input directory"`
}

func (c putCmd) Run(g globalCmd, args []string) error {
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

	// list files
	for _, arg := range args {
		if !filepath.IsAbs(arg) {
			arg = filepath.Join(c.InputSrc, arg)
		}
		ff, err := zglob.Glob(arg)
		if err != nil {
			return err
		}

		for _, f := range ff {
			fmt.Fprintf(os.Stderr, "putting: %v\n", f)

			file, err := os.Open(f)
			if err != nil {
				return fmt.Errorf("open %v: %v", f, err)
			}
			content, err := ioutil.ReadAll(file)
			if err != nil {
				return fmt.Errorf("read %v: %v", f, err)
			}
			file.Close()

			filename := filepath.Base(f)
			extlen := len(filepath.Ext(filename))

			// find messages
			{
				msgService := gmail.NewUsersMessagesService(gmailService)
				resp, err := msgService.List(g.UserID).LabelIds(pomeraSync.Id).Q("subject:(" + filename[:len(filename)-extlen] + ")").Do()
				if err != nil {
					return err
				}
				if len(resp.Messages) > 0 {
					_, err := msgService.Trash(g.UserID, resp.Messages[0].Id).Do()
					if err != nil {
						return err
					}
				}
			}

			subject := base64.StdEncoding.EncodeToString([]byte(filename[:len(filename)-extlen]))
			if err != nil {
				fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
				break //continue
			}

			msg := gmail.Message{
				LabelIds: []string{pomeraSync.Id},
				Raw: base64.URLEncoding.EncodeToString([]byte("Content-Type: text/plain; charset=\"utf-8-sig\"\r\n" +
					"MIME-Version: 1.0\r\n" +
					"Content-Transfer-Encoding: base64\r\n" +
					"X-Uniform-Type-Identifier: com.apple.mail-note\r\n" +
					"From: " + g.UserID + "\r\n" +
					"Subject: =?UTF-8?B?" + subject + "?=\r\n" +
					"Date: " + time.Now().Format(time.RFC822Z) + "\r\n" +
					"\r\n" +
					base64.StdEncoding.EncodeToString(content))),
			}

			msgService := gmail.NewUsersMessagesService(gmailService)
			_, err = msgService.Insert(g.UserID, &msg).Do()
			if err != nil {
				return err
			}
		}
	}

	/*
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
					println("ERROR: " + err.Error())
					break //continue
				}
				content = string(decoded)

				if c.OutputTarget == "file" {
					name := c.OutputFormat
					if strings.Index(c.OutputFormat, "{subject}") != -1 {
						name = strings.Replace(name, "{subject}", getHeader(m.Payload.Headers, "Subject"), -1)
					}

					if c.OutputDest != "" {
						name = filepath.Join(c.OutputDest, name)
					}

					file, err := os.Create(name)
					if err != nil {
						return fmt.Errorf("create %v: %v", name, err)
					}
					file.WriteString(content)
					file.Close()
				} else {
					println(content)
				}
			}
		}
	*/

	return nil
}
