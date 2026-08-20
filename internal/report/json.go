package report

import (
	"encoding/json"
	"time"

	"github.com/NovaLux12/dig/internal/analyze"
	"github.com/NovaLux12/dig/internal/git"
)

// RenderJSON returns a machine-readable JSON representation of the report.
// It uses stdlib encoding/json only, emits RFC3339 timestamps, and ensures
// slices are [] rather than null for stable downstream parsing.
// When d is nil the delta field is omitted.
func RenderJSON(r *analyze.Report, d *analyze.Delta) ([]byte, error) {
	if r == nil {
		return nil, nil
	}
	out := jsonReport{
		RepoName:         r.RepoName,
		RepoPath:         r.RepoPath,
		Accent:           r.Accent,
		GeneratedAt:      r.GeneratedAt.UTC().Format(time.RFC3339),
		TotalCommits:     r.TotalCommits,
		ContributorCount: len(r.Contributors),
		FileCount:        r.FileCount,
		FirstAt:          r.FirstCommit.Time.UTC().Format(time.RFC3339),
		LastAt:           r.LastCommit.Time.UTC().Format(time.RFC3339),
		BusFactor:        r.BusFactor,
		BusFactorMsg:     r.BusFactorMsg,
		Timeline:         toJSONTimeline(r.Timeline),
		Contributors:     toJSONContributors(r.Contributors),
		HotFiles:         toJSONHotFiles(r.HotFiles),
		Languages:        toJSONLangs(r.Languages),
		FirstCommit:      toJSONCommit(r.FirstCommit),
		LastCommit:       toJSONCommit(r.LastCommit),
		Readme:           r.Readme,
	}
	if p, n := r.PeakMonth(); p != "" || n != 0 {
		out.PeakLabel = p
		out.PeakCommits = n
	}
	if months, total := r.MonthsSpan(); months != 0 {
		out.Months = months
		out.TimelineTotal = total
	}
	if d != nil {
		jd := &jsonDelta{
			BaseRef:              d.BaseRef,
			TargetRef:            d.TargetRef,
			CommitDelta:          d.CommitDelta,
			ContribDelta:         d.ContribDelta,
			BusFactorDelta:       d.BusFactorDelta,
			BusFactorMsg:         d.BusFactorMsg,
			CommitsAdded:         toJSONCommits(d.CommitsAdded),
			CommitsRemoved:       toJSONCommits(d.CommitsRemoved),
			NewContributors:      toJSONContributors(d.NewContributors),
			DepartedContributors: toJSONContributors(d.DepartedContributors),
			NewHotFiles:          ensureSlice(d.NewHotFiles),
			LostHotFiles:         ensureSlice(d.LostHotFiles),
			LanguageGrowth:       toJSONLangGrowth(d.LanguageGrowth),
		}
		out.Delta = jd
	}
	return json.MarshalIndent(out, "", "  ")
}

type jsonReport struct {
	RepoName         string            `json:"repoName"`
	RepoPath         string            `json:"repoPath"`
	Accent           string            `json:"accent"`
	GeneratedAt      string            `json:"generatedAt"`
	TotalCommits     int               `json:"totalCommits"`
	ContributorCount int               `json:"contributorCount"`
	FileCount        int               `json:"fileCount"`
	FirstAt          string            `json:"firstAt"`
	LastAt           string            `json:"lastAt"`
	BusFactor        int               `json:"busFactor"`
	BusFactorMsg     string            `json:"busFactorMsg"`
	Months           int               `json:"months"`
	TimelineTotal    int               `json:"timelineTotal"`
	PeakLabel        string            `json:"peakLabel"`
	PeakCommits      int               `json:"peakCommits"`
	Timeline         []jsonMonth       `json:"timeline"`
	Contributors     []jsonContributor `json:"contributors"`
	HotFiles         []jsonHotFile     `json:"hotFiles"`
	Languages        []jsonLang        `json:"languages"`
	FirstCommit      jsonCommit        `json:"firstCommit"`
	LastCommit       jsonCommit        `json:"lastCommit"`
	Readme           string            `json:"readme"`
	Delta            *jsonDelta        `json:"delta,omitempty"`
}

type jsonMonth struct {
	Year    int    `json:"year"`
	Month   int    `json:"month"`
	Label   string `json:"label"`
	Commits int    `json:"commits"`
}

type jsonContributor struct {
	Name    string `json:"name"`
	Email   string `json:"email"`
	Commits int    `json:"commits"`
	FirstAt string `json:"firstAt"`
	LastAt  string `json:"lastAt"`
}

type jsonHotFile struct {
	Path          string `json:"path"`
	Touches       int    `json:"touches"`
	LastModified  string `json:"lastModified"`
	PrimaryAuthor string `json:"primaryAuthor"`
}

type jsonLang struct {
	Extension string `json:"extension"`
	Files     int    `json:"files"`
	Lines     int64  `json:"lines"`
}

type jsonCommit struct {
	Hash    string          `json:"hash"`
	Author  string          `json:"author"`
	Email   string          `json:"email"`
	Time    string          `json:"time"`
	Subject string          `json:"subject"`
	Body    string          `json:"body"`
	Parents []string        `json:"parents"`
	Files   []jsonFileTouch `json:"files"`
}

type jsonFileTouch struct {
	Path    string `json:"path"`
	Added   int    `json:"added"`
	Deleted int    `json:"deleted"`
}

type jsonDelta struct {
	BaseRef              string            `json:"baseRef"`
	TargetRef            string            `json:"targetRef"`
	CommitDelta          int               `json:"commitDelta"`
	ContribDelta         int               `json:"contribDelta"`
	BusFactorDelta       int               `json:"busFactorDelta"`
	BusFactorMsg         string            `json:"busFactorMsg"`
	CommitsAdded         []jsonCommit      `json:"commitsAdded"`
	CommitsRemoved       []jsonCommit      `json:"commitsRemoved"`
	NewContributors      []jsonContributor `json:"newContributors"`
	DepartedContributors []jsonContributor `json:"departedContributors"`
	NewHotFiles          []string          `json:"newHotFiles"`
	LostHotFiles         []string          `json:"lostHotFiles"`
	LanguageGrowth       []jsonLangGrowth  `json:"languageGrowth"`
}

type jsonLangGrowth struct {
	Extension   string `json:"extension"`
	BaseLines   int64  `json:"baseLines"`
	TargetLines int64  `json:"targetLines"`
	Delta       int64  `json:"delta"`
}

func toJSONTimeline(in []analyze.MonthBucket) []jsonMonth {
	if len(in) == 0 {
		return []jsonMonth{}
	}
	out := make([]jsonMonth, 0, len(in))
	for _, b := range in {
		out = append(out, jsonMonth{
			Year:    b.Year,
			Month:   int(b.Month),
			Label:   b.Label,
			Commits: b.Commits,
		})
	}
	return out
}

func toJSONContributors(in []git.Contributor) []jsonContributor {
	if len(in) == 0 {
		return []jsonContributor{}
	}
	out := make([]jsonContributor, 0, len(in))
	for _, c := range in {
		out = append(out, jsonContributor{
			Name:    c.Name,
			Email:   c.Email,
			Commits: c.Commits,
			FirstAt: c.FirstAt.UTC().Format(time.RFC3339),
			LastAt:  c.LastAt.UTC().Format(time.RFC3339),
		})
	}
	return out
}

func toJSONHotFiles(in []git.FileStat) []jsonHotFile {
	if len(in) == 0 {
		return []jsonHotFile{}
	}
	out := make([]jsonHotFile, 0, len(in))
	for _, f := range in {
		out = append(out, jsonHotFile{
			Path:          f.Path,
			Touches:       f.Touches,
			LastModified:  f.LastModified.UTC().Format(time.RFC3339),
			PrimaryAuthor: f.PrimaryAuthor,
		})
	}
	return out
}

func toJSONLangs(in []git.LangStat) []jsonLang {
	if len(in) == 0 {
		return []jsonLang{}
	}
	out := make([]jsonLang, 0, len(in))
	for _, l := range in {
		out = append(out, jsonLang{
			Extension: l.Extension,
			Files:     l.Files,
			Lines:     l.Lines,
		})
	}
	return out
}

func toJSONCommit(c git.Commit) jsonCommit {
	parents := ensureSlice(c.Parents)
	files := []jsonFileTouch{}
	if len(c.Files) > 0 {
		files = make([]jsonFileTouch, 0, len(c.Files))
		for _, f := range c.Files {
			files = append(files, jsonFileTouch{
				Path:    f.Path,
				Added:   f.Added,
				Deleted: f.Deleted,
			})
		}
	}
	return jsonCommit{
		Hash:    c.Hash,
		Author:  c.Author,
		Email:   c.Email,
		Time:    c.Time.UTC().Format(time.RFC3339),
		Subject: c.Subject,
		Body:    c.Body,
		Parents: parents,
		Files:   files,
	}
}

func toJSONCommits(in []git.Commit) []jsonCommit {
	if len(in) == 0 {
		return []jsonCommit{}
	}
	out := make([]jsonCommit, 0, len(in))
	for _, c := range in {
		out = append(out, toJSONCommit(c))
	}
	return out
}

func toJSONLangGrowth(in []analyze.LanguageChange) []jsonLangGrowth {
	if len(in) == 0 {
		return []jsonLangGrowth{}
	}
	out := make([]jsonLangGrowth, 0, len(in))
	for _, lg := range in {
		out = append(out, jsonLangGrowth{
			Extension:   lg.Extension,
			BaseLines:   lg.BaseLines,
			TargetLines: lg.TargetLines,
			Delta:       lg.Delta,
		})
	}
	return out
}

func ensureSlice[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}
