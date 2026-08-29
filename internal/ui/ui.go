package ui

import "html/template"

type LandingData struct {
	InboxID  string
	InboxURL string
}

type InboxData struct {
	ID string
}

var Landing = template.Must(template.New("landing").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>hookd</title>
</head>
<body>
<h1>hookd</h1>
<p>Local webhook inbox. Paste this URL into Stripe, GitHub, or curl:</p>
<p><code>{{.InboxURL}}</code></p>
<p>Inspect captured requests: <a href="/i/{{.InboxID}}/requests"><code>/i/{{.InboxID}}/requests</code></a></p>
<form method="post" action="/inboxes">
<button type="submit">New inbox</button>
</form>
</body>
</html>
`))

var Inbox = template.Must(template.New("inbox").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>hookd inbox {{.ID}}</title>
</head>
<body>
<h1>inbox {{.ID}}</h1>
<p>Placeholder UI. Captured requests:</p>
<p><a href="/i/{{.ID}}/requests"><code>/i/{{.ID}}/requests</code></a></p>
<p><a href="/">Home</a></p>
</body>
</html>
`))
