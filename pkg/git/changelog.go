package git

import (
	"fmt"
	"github.com/naveego/bosun/pkg/issues"
	"regexp"
	"strconv"
	"strings"
)

type GitChange struct {
	Valid          bool
	Title          string
	Body           string
	Date           string
	Issue          string
	CommitID       string
	CommitType     string
	Committer      string
	BreakingChange bool
	StoryLink      string
	IssueLink      string
}

type GitChangeLog struct {
	VersionBump   string
	Changes       GitChanges
	OutputMessage string
}

type GitChangeLogOptions struct {
	Description bool
	UnknownType bool
}

const (
	StateLookingForCommitNumber = iota
	StateLookingForAuthor
	StateLookingForDate
	StateLookingForTitle
	StateLookingForBody
)

var allTypes = []string{"feat", "perf", "test", "chore", "fix", "docs", "style", "refactor"}
var allCommitTypeString = strings.Join(allTypes, "|")

// TODO: imporve Regex with $ and ^ to be robust
var RegexMatchCommitID = regexp.MustCompile(`commit .{40}`)
var RegexMatchFormattedTitle = regexp.MustCompile(`(` + allCommitTypeString + `)(\([^)]+\))?:(.*)`)
var RegexMatchAlternativeTitle = regexp.MustCompile(`^\* .*`)
var RegexMatchDate = regexp.MustCompile(`Date: .*`)
var RegexMatchAuthor = regexp.MustCompile(`Author: .*`)
var RegexMatchIssue = regexp.MustCompile(`\s+(resolves )?(@|#)?[0-9]+$|^[0-9]+$`)

var RegexGetAuthor = regexp.MustCompile(`Author: `)
var RegexGetCommitType = regexp.MustCompile(`(` + allCommitTypeString + `)`)
var RegexGetCommitId = regexp.MustCompile(` .{40}`)
var RegexGetDate = regexp.MustCompile(`Date: `)
var RegexGetIssue = regexp.MustCompile(`[0-9]+$`)

const Major = "major"
const Minor = "minor"
const Patch = "patch"
const Unknown = "unknown"

type GitChanges []GitChange

var skipKeys = []*regexp.Regexp{
	regexp.MustCompile(`^\s*$`),
}

var bumpMap = map[string]string{"fix": Patch, "docs": Patch, "style": Patch, "refactor": Patch,
	"feat": Minor, "perf": Minor, "test": Minor, "chore": Minor}

func (g GitWrapper) ChangeLog(from, to string, svc issues.IssueService, options GitChangeLogOptions) (GitChangeLog, error) {
	out, err := g.Exec("log", fmt.Sprintf("%s..%s", to, from))
	owner, repo := GetOrgAndRepoFromPath(g.dir)

	var allChanges = GitChanges{}
	var committer string
	var commitId string
	var title string
	var date string
	var bodyBuilder = strings.Builder{}
	var commitType string
	var state = StateLookingForCommitNumber
	lines := strings.Split(out, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for _, skipper := range skipKeys {
			if skipper.MatchString(line) {
				goto End
			}
		}

		switch state {
		case StateLookingForCommitNumber:
			commitIdMatch := RegexMatchCommitID.FindStringSubmatch(line)
			if len(commitIdMatch) > 0 {
				commitId = RegexGetCommitId.FindString(commitIdMatch[0])
				state = StateLookingForAuthor
				continue
			}

		case StateLookingForAuthor:
			committerMatch := RegexMatchAuthor.FindStringSubmatch(line)
			if len(committerMatch) > 0 {
				committer = RegexGetAuthor.Split(committerMatch[0], -1)[1]
				state = StateLookingForDate
				continue
			}

		case StateLookingForDate:
			dateMatch := RegexMatchDate.FindStringSubmatch(line)
			if len(dateMatch) > 0 {
				date = RegexGetDate.Split(dateMatch[0], -1)[1]
				state = StateLookingForTitle
				continue
			}
		case StateLookingForTitle:
			titleMatch := RegexMatchFormattedTitle.FindString(line)
			commitIdMatch := RegexMatchCommitID.FindStringSubmatch(line)
			if len(commitIdMatch) > 0 {
				commitId = RegexGetCommitId.FindString(commitIdMatch[0])
				bodyBuilder.Reset()
				state = StateLookingForAuthor

			} else if len(titleMatch) > 0 {
				title = line
				commitType = RegexGetCommitType.FindString(title)
				state = StateLookingForBody

			} else {
				// Line is not a standard commit
				change := GitChange{
					CommitID:  commitId,
					Committer: committer,
					Body:      CleanFrontAsterisk(line),
					Date:      date,
					Valid:     false,
				}
				allChanges = append(allChanges, change)
				state = StateLookingForTitle

			}
			continue
		case StateLookingForBody:
			commitIdMatch := RegexMatchCommitID.FindStringSubmatch(line)
			titleMatch := RegexMatchFormattedTitle.FindString(line)
			alternativeTitleMatch := RegexMatchAlternativeTitle.FindString(line)
			issueMatch := RegexMatchIssue.MatchString(line)
			if len(commitIdMatch) > 0 {
				change := GitChange{
					CommitID:       commitId,
					Committer:      committer,
					CommitType:     commitType,
					Title:          CleanFrontAsterisk(title),
					Body:           bodyBuilder.String(),
					Date:           date,
					Valid:          true,
					BreakingChange: CommitHasBreakingChange(bodyBuilder.String()),
				}
				allChanges = append(allChanges, change)
				commitId = RegexGetCommitId.FindString(commitIdMatch[0])
				bodyBuilder.Reset()
				state = StateLookingForAuthor

			} else if issueMatch {

				issue := RegexGetIssue.FindString(line)
				issueLink := GetIssueLink(g.dir, issue)
				var storyLink string
				if svc != nil {
					storyLink, _ = GetStoryLink(svc, owner, repo, issue)
				}
				change := GitChange{
					CommitID:       commitId,
					Committer:      committer,
					CommitType:     commitType,
					Title:          CleanFrontAsterisk(title),
					Body:           bodyBuilder.String(),
					Date:           date,
					Valid:          true,
					IssueLink:      issueLink,
					Issue:          issue,
					StoryLink:      storyLink,
					BreakingChange: CommitHasBreakingChange(bodyBuilder.String()),
				}
				allChanges = append(allChanges, change)
				bodyBuilder.Reset()
				state = StateLookingForTitle

			} else if len(titleMatch) > 0 {
				change := GitChange{
					CommitID:       commitId,
					Committer:      committer,
					CommitType:     commitType,
					Title:          CleanFrontAsterisk(title),
					Body:           bodyBuilder.String(),
					Date:           date,
					Valid:          true,
					BreakingChange: CommitHasBreakingChange(bodyBuilder.String()),
				}
				allChanges = append(allChanges, change)
				title = RegexMatchFormattedTitle.FindString(line)
				bodyBuilder.Reset()
				state = StateLookingForBody

			} else if len(alternativeTitleMatch) > 0 {
				change := GitChange{
					CommitID:  commitId,
					Committer: committer,
					Body:      CleanFrontAsterisk(line),
					Date:      date,
					Valid:     false,
				}
				allChanges = append(allChanges, change)
				state = StateLookingForTitle
			} else {
				fmt.Fprintf(&bodyBuilder, "%s\n", line)
			}
			continue
		}

	End:
	}

	changeLog := GitChangeLog{
		VersionBump:   allChanges.GetVersionBump(),
		Changes:       allChanges,
		OutputMessage: GetChangeLogOutputMessage(allChanges.GetVersionBump(), allChanges, options),
	}
	return changeLog, err
}

func CommitHasBreakingChange(body string) bool {
	breakingChangeFormat := regexp.MustCompile(`BREAKING CHANGE: .*`)
	return breakingChangeFormat.MatchString(body)
}

func (g GitChanges) GetVersionBump() string {
	changes := g.GetSeparatedChanges()

	if len(changes[Major]) > 0 {
		return "MAJOR"
	} else if len(changes[Minor]) > 0 {
		return "MINOR"
	} else if len(changes[Patch]) > 0 {
		return "PATCHES"
	}
	return "NONE"
}

func (g GitChanges) GetSeparatedChanges() map[string]GitChanges {
	storeChange := make(map[string]GitChanges)
	for _, change := range g {
		var bump string
		var known bool
		if change.BreakingChange {
			bump = Major
		} else if bump, known = bumpMap[change.CommitType]; !known {
			bump = Unknown
		}
		storeChange[bump] = append(storeChange[bump], change)
	}
	return storeChange
}

func GetStoryLink(svc issues.IssueService, owner string, repo string, issue string) (string, error) {
	var builder = strings.Builder{}
	issueNum, err := strconv.Atoi(issue)
	if err != nil {
		return "", err
	}
	iss, err := svc.GetParents(issues.NewIssueRef(owner, repo, issueNum))
	if err != nil {
		return "", err
	}
	if len(iss) != 2 {
		// this means the issue number must be wrong
		return "", err
	}
	fmt.Fprintf(&builder, "https://github.com/%s/%s/issues/%s", iss[1].Org, iss[1].Repo, strconv.Itoa(iss[1].Number))

	return builder.String(), err
}

func GetIssueLink(dir string, issue string) string {
	linkFormat := regexp.MustCompile(`github\.com.*`)
	var builder = strings.Builder{}
	fmt.Fprintf(&builder, "https://%s/issues/%s", linkFormat.FindString(dir), issue)
	return builder.String()
}

func GetChangeLogOutputMessage(bump string, changes GitChanges, options GitChangeLogOptions) string {
	allChanges := changes.GetSeparatedChanges()
	output := new(strings.Builder)
	fmt.Fprintf(output, "Recommended Version bump: %s\n\n", bump)
	fmt.Fprintf(output, "Major: %d\n", len(allChanges[Major]))
	fmt.Fprintf(output, "Minor: %d\n", len(allChanges[Minor]))
	fmt.Fprintf(output, "Patch: %d\n", len(allChanges[Patch]))
	fmt.Fprintf(output, "Unknown: %d\n", len(allChanges[Unknown]))
	if len(allChanges[Major]) != 0 {
		fmt.Fprintf(output, "\nMajor Changes: \n")
		for _, changes := range allChanges[Major] {
			fmt.Fprintf(output, BuildOutput(changes, options))
		}
	}

	if len(allChanges[Minor]) != 0 {
		fmt.Fprintf(output, "\nMinor Changes: \n")
		for _, changes := range allChanges[Minor] {
			fmt.Fprintf(output, BuildOutput(changes, options))
		}
	}

	if len(allChanges[Patch]) != 0 {
		fmt.Fprintf(output, "\nPatch Changes: \n")
		for _, changes := range allChanges[Patch] {
			fmt.Fprintf(output, BuildOutput(changes, options))
		}
	}

	if len(allChanges[Unknown]) != 0 && options.Description {
		fmt.Fprintf(output, "\nUnknown Changes: \n")
		for _, changes := range allChanges[Unknown] {
			BuildUnknownOutput(output, changes)
		}
	}

	return output.String()
}

func BuildOutput(changes GitChange, options GitChangeLogOptions) string {
	var builder = strings.Builder{}
	var bodyArr []string
	fmt.Fprintf(&builder, "\t%s, by %s\n", changes.Title, changes.Committer)
	if options.Description {
		if len(changes.Body) > 0 {
			var formattedBody string
			if len(changes.Body) > 50 {
				formattedBody = LongBodyFormatter(changes.Body)
			} else {
				formattedBody = changes.Body
			}
			bodyArr = strings.Split(formattedBody, "\n")
			body := strings.Join(bodyArr[:len(bodyArr)-1], "\n\t\t")

			fmt.Fprintf(&builder, "\t\t%s\n", body)
		}

		if len(changes.StoryLink) > 0 {
			fmt.Fprintf(&builder, "\n\t\tIssue: %s   Story: %s\n", changes.IssueLink, changes.StoryLink)
		}
	}
	return strings.Replace(builder.String(), "\n\n", "\n", -1)
}

func BuildUnknownOutput(builder *strings.Builder, changes GitChange) {
	fmt.Fprintf(builder, "\t%s\n", changes.Body)
}

func LongBodyFormatter(body string) string {
	var bodyBuilder = strings.Builder{}
	var tooLong = false
	var letterCount = 1
	var removedNewLinesBody = strings.Join(strings.Split(body, "\n"), " ")
	for _, c := range removedNewLinesBody {
		fmt.Fprintf(&bodyBuilder, "%c", c)
		if (c > 'a' && c < 'z') || (c > 'A' && c < 'Z') {
			letterCount++
		}
		if letterCount%50 == 0 {
			tooLong = true
		}
		if tooLong && c == ' ' {
			fmt.Fprintf(&bodyBuilder, "\n")
			tooLong = false
		}
	}
	return bodyBuilder.String()

}

func CleanFrontAsterisk(title string) string {
	titleMatch := RegexMatchAlternativeTitle.FindString(title)
	if len(titleMatch) > 0 {
		return strings.Replace(title, "* ", "", 1)
	}
	return title
}
