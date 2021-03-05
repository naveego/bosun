package git

import (
	"fmt"
	"github.com/naveego/bosun/pkg/issues"
	"github.com/naveego/bosun/pkg/semver"
	"regexp"
	"strings"
)

func (g GitChange) Format(f fmt.State, c rune) {
	switch c {
	case 's':
		_, _ = f.Write([]byte(g.Title))
	default:
		_, _ = f.Write([]byte("change"))
	}
}

type GitChangeLog struct {
	VersionBump   semver.Bump
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
var bumpMap = map[string]semver.Bump{
	"feat":              semver.BumpMinor,
	"fix":               semver.BumpPatch,
	"refactor":          semver.BumpPatch,
	"perf":              semver.BumpPatch,
	"docs":              semver.BumpNone,
	"style":             semver.BumpNone,
	"test":              semver.BumpNone,
	"chore":             semver.BumpNone,
	"deploy":            semver.BumpPatch,
	MalformedCommitFlag: semver.BumpPatch,
}

func init(){
	for k := range bumpMap {
		allTypes = append(allTypes, k)
	}
}

var allTypes []string
var allCommitTypeString = strings.Join(allTypes, "|")

// TODO: imporve Regex with $ and ^ to be robust
var RegexMatchCommitID = regexp.MustCompile(`commit .{40}`)
var RegexMatchFormattedTitle = regexp.MustCompile(`(` + allCommitTypeString + `)(\([^)]+\))?:(.*)`)
var RegexMatchAlternativeTitle = regexp.MustCompile(`^\* .*`)
var RegexMatchDate = regexp.MustCompile(`Date: .*`)
var RegexMatchAuthor = regexp.MustCompile(`Author: ([^<]+)\s+<([^>]+)>`)
var RegexMatchIssue = regexp.MustCompile(`\s+(resolves )?(@|#)?[0-9]+$|^[0-9]+$`)

var RegexGetCommitType = regexp.MustCompile(`(` + allCommitTypeString + `)`)
var RegexGetCommitId = regexp.MustCompile(` .{40}`)
var RegexGetDate = regexp.MustCompile(`Date: `)
var RegexGetIssue = regexp.MustCompile(`[0-9]+$`)

var skipKeys = []*regexp.Regexp{
	regexp.MustCompile(`^\s*$`),
}



const MalformedCommitFlag = "malformed"

func (g GitWrapper) ChangeLog(notInBranch, inBranch string, svc issues.IssueService, options GitChangeLogOptions) (GitChangeLog, error) {
	out, err := g.Exec("log", "--no-merges", fmt.Sprintf("%s..%s", inBranch, notInBranch))

	org, repo := GetRepoRefFromPath(g.dir).OrgAndRepo()
	var allChanges = GitChanges{}
	var committer string
	var commitId string
	var title string
	var date string
	var bodyBuilder = strings.Builder{}
	var commitType string
	var state = StateLookingForCommitNumber
	lines := strings.Split(out, "\n")
	for i := 0; i < len(lines); i++ {
		line := lines[i]
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
				committer = committerMatch[1]
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

			} else if strings.HasPrefix(line, "Merge") {
				// is a merge commit, we don't care
				continue
			} else {
				// Line is not a standard commit
				commitType = MalformedCommitFlag
				title = fmt.Sprintf("%s: %s", MalformedCommitFlag, line)
				state = StateLookingForBody
				continue
			}
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

				issueNumber := RegexGetIssue.FindString(line)
				issue := issues.NewIssueRef(org, repo, issueNumber)
				issueLink := GetIssueLink(g.dir, issue.String())

				change := GitChange{
					CommitID:       commitId,
					Committer:      committer,
					CommitType:     commitType,
					Title:          CleanFrontAsterisk(title),
					Body:           bodyBuilder.String(),
					Date:           date,
					Valid:          true,
					IssueLink:      issueLink,
					Issue:          &issue,
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

func (g GitChanges) GetVersionBump() semver.Bump {
	changes := g.GetSeparatedChanges()

	if len(changes[semver.BumpMajor]) > 0 {
		return semver.BumpMajor
	} else if len(changes[semver.BumpMinor]) > 0 {
		return semver.BumpMinor
	} else if len(changes[semver.BumpPatch]) > 0 {
		return semver.BumpPatch
	}
	return semver.BumpNone
}

func (g GitChanges) GetSeparatedChanges() map[semver.Bump]GitChanges {
	storeChange := make(map[semver.Bump]GitChanges)
	for _, change := range g {
		var bump semver.Bump
		var known bool
		if change.BreakingChange {
			bump = semver.BumpMajor
		} else if bump, known = bumpMap[change.CommitType]; !known {
			bump = semver.Unknown
		}
		storeChange[bump] = append(storeChange[bump], change)
	}
	return storeChange
}

func GetIssueLink(dir string, issue string) string {
	linkFormat := regexp.MustCompile(`github\.com.*`)
	var builder = strings.Builder{}
	fmt.Fprintf(&builder, "https://%s/issues/%s", linkFormat.FindString(dir), issue)
	return builder.String()
}

func GetChangeLogOutputMessage(bump semver.Bump, c GitChanges, options GitChangeLogOptions) string {
	allChanges := c.GetSeparatedChanges()
	output := new(strings.Builder)
	_, _ = fmt.Fprintf(output, "Recommended Version bump: %s\n\n", bump)
	_, _ = fmt.Fprintf(output, "Major: %d\n", len(allChanges[semver.BumpMajor]))
	_, _ = fmt.Fprintf(output, "Minor: %d\n", len(allChanges[semver.BumpMinor]))
	_, _ = fmt.Fprintf(output, "Patch: %d\n", len(allChanges[semver.BumpPatch]))
	_, _ = fmt.Fprintf(output, "Unknown: %d\n", len(allChanges[semver.Unknown]))
	if len(allChanges[semver.BumpMajor]) != 0 {
		fmt.Fprintf(output, "\nMajor Changes: \n")
		for _, changes := range allChanges[semver.BumpMajor] {
			fmt.Fprintf(output, BuildOutput(changes, options))
		}
	}

	if len(allChanges[semver.BumpMinor]) != 0 {
		fmt.Fprintf(output, "\nMinor Changes: \n")
		for _, changes := range allChanges[semver.BumpMinor] {
			fmt.Fprintf(output, BuildOutput(changes, options))
		}
	}

	if len(allChanges[semver.BumpPatch]) != 0 {
		fmt.Fprintf(output, "\nPatch Changes: \n")
		for _, changes := range allChanges[semver.BumpPatch] {
			fmt.Fprintf(output, BuildOutput(changes, options))
		}
	}

	if len(allChanges[semver.Unknown]) != 0 && options.Description {
		fmt.Fprintf(output, "\nUnknown Changes: \n")
		for _, changes := range allChanges[semver.Unknown] {
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
