package timatch

import (
	"strings"
	"text/template"
)

var tmplMatchesDrafting = template.Must(template.New("MatchesDrafting").Parse(strings.TrimSpace(`
{{ range . }}
In Drafting: {{ .RadiantTeam.TeamName }} vs. {{ .DireTeam.TeamName }} (Game {{ .GameNumber }})
{{- end -}}
`)))

var tmplMatchesStarted = template.Must(template.New("MatchesStarted").Parse(strings.TrimSpace(`
{{ range . }}
Match Started: {{ .RadiantTeam.TeamName }} vs. {{ .DireTeam.TeamName }} (Game {{ .GameNumber }})
{{- end -}}
`)))

type matchesFinishedDataItem struct {
	GameNumber  int
	WinnerName  string
	LoserName   string
	WinnerScore int
	LoserScore  int
}

var tmplMatchesFinished = template.Must(template.New("MatchesFinished").Parse(strings.TrimSpace(`
{{ range . }}
Match Ended: {{ .WinnerName }} defeated {{ .LoserName }} ({{ .WinnerScore }} - {{ .LoserScore }}, Game {{ .GameNumber }})
{{- end -}}
`)))
