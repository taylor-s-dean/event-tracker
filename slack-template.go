package main

import "text/template"

var slackTemplate = template.Must(template.New("").Parse(`
{{if eq .EventType "PULL REQUEST"}}
*PR merged into {{.Metadata.repository.full_name}} by {{.Metadata.pull_request.user.login}} at {{.StartTime.Format "Mon, 02 Jan 2006 15:04:05 MST"}}*
<{{.Metadata.pull_request.html_url}}|{{.Metadata.pull_request.title}}>
{{.Metadata.pull_request.body}}
{{else}}
` + "```{{.MarshalString}}```" + `
{{end}}
`))
