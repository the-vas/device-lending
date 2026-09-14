package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"net/mail"

	"github.com/pocketbase/pocketbase/core"
	pbmailer "github.com/pocketbase/pocketbase/tools/mailer"
)

type Notifier struct {
	app     core.App
	baseURL string
}

func New(app core.App, baseURL string) *Notifier {
	return &Notifier{app: app, baseURL: baseURL}
}

var bodyTmpl = template.Must(template.New("email").Parse(
	`<p>{{.Intro}}</p>{{if .Detail}}<p>{{.Detail}}</p>{{end}}<p><a href="{{.Link}}">View on the portal</a></p>`,
))

type bodyData struct {
	Intro  string
	Detail string
	Link   string
}

func (n *Notifier) render(intro, detail, link string) (string, error) {
	var buf bytes.Buffer
	if err := bodyTmpl.Execute(&buf, bodyData{Intro: intro, Detail: detail, Link: link}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (n *Notifier) send(to *core.Record, subject, intro, detail, link string) error {
	html, err := n.render(intro, detail, link)
	if err != nil {
		return err
	}

	settings := n.app.Settings()
	message := &pbmailer.Message{
		From:    mail.Address{Address: settings.Meta.SenderAddress, Name: settings.Meta.SenderName},
		To:      []mail.Address{{Address: to.Email(), Name: to.GetString("name")}},
		Subject: subject,
		HTML:    html,
	}
	return n.app.NewMailClient().Send(message)
}

func deviceLink(baseURL string, device *core.Record) string {
	return fmt.Sprintf("%s/devices/%s", baseURL, device.Id)
}

func (n *Notifier) NewRequest(owner, device, requester *core.Record) error {
	intro := fmt.Sprintf("%s requested to borrow %q.", requester.GetString("name"), device.GetString("name"))
	return n.send(owner, "New borrow request for "+device.GetString("name"), intro, "", deviceLink(n.baseURL, device))
}

func (n *Notifier) RequestRejected(requester, device *core.Record) error {
	intro := fmt.Sprintf("Your request to borrow %q was declined.", device.GetString("name"))
	return n.send(requester, "Request declined: "+device.GetString("name"), intro, "", deviceLink(n.baseURL, device))
}

func (n *Notifier) HandoverAccepted(requester, device *core.Record, lendEnd string) error {
	intro := fmt.Sprintf("%q is ready for you to pick up.", device.GetString("name"))
	detail := ""
	if lendEnd != "" {
		detail = "Expected return date: " + lendEnd
	}
	return n.send(requester, "You're getting "+device.GetString("name"), intro, detail, deviceLink(n.baseURL, device))
}

func (n *Notifier) DeviceUnavailable(requester, device *core.Record, lendEnd string, otherPendingCount int) error {
	intro := fmt.Sprintf("%q was just lent to someone else and is no longer available for now.", device.GetString("name"))
	detail := fmt.Sprintf("%d other request(s) besides yours are still waiting for it.", otherPendingCount)
	if lendEnd != "" {
		detail = "Expected back: " + lendEnd + ". " + detail
	}
	return n.send(requester, device.GetString("name")+" is now unavailable", intro, detail, deviceLink(n.baseURL, device))
}

func (n *Notifier) DeviceAvailableAgain(requester, device *core.Record) error {
	intro := fmt.Sprintf("%q was returned and is available again.", device.GetString("name"))
	return n.send(requester, device.GetString("name")+" is available again", intro, "", deviceLink(n.baseURL, device))
}

func (n *Notifier) RequestWithdrawn(owner, device, requester *core.Record) error {
	intro := fmt.Sprintf("%s withdrew their request to borrow %q.", requester.GetString("name"), device.GetString("name"))
	return n.send(owner, "Request withdrawn: "+device.GetString("name"), intro, "", deviceLink(n.baseURL, device))
}

func (n *Notifier) DeviceRemoved(requester, device *core.Record) error {
	intro := fmt.Sprintf("%q was removed by its owner and is no longer available.", device.GetString("name"))
	return n.send(requester, device.GetString("name")+" was removed", intro, "", n.baseURL)
}
