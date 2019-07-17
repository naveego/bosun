package git

import (
	"fmt"
	"github.com/naveego/bosun/pkg"
	"github.com/naveego/bosun/pkg/issues"
	"regexp"
	"strconv"
	"strings"
)

type GitChange struct {
	Valid          bool
	Title          string
	Body           string
	Issue          string
	CommitType     string
	Committer      string
	BreakingChange bool
	StoryLink      string
	IssueLink      string
}

type GitChangeLog struct {
	VersionBump string
	Changes     GitChanges
	OutputMessage string
}

type GitChangeLogOptions struct {
	Description bool
	UnknownType bool
}

type GitChanges []GitChange

var minorTypes = map[string]bool{"feat": true, "perf": true, "test": true, "chore": true}
var patchTypes = map[string]bool{"fix": true, "docs": true, "style": true, "refactor": true}

func (g GitWrapper) ChangeLog(logPath string, command string, svc issues.IssueService, options GitChangeLogOptions) (GitChangeLog, error) {
	args := append([]string{"-C", g.dir, "log"}, logPath, command)
	out, err := pkg.NewCommand("git", args...).RunOut()
	commits := regexp.MustCompile(`commit .{40}`).Split(out, -1)
	repo := GetRepo(g.dir)
	owner := GetOwner(g.dir)
	gitChangesHolder := make([]GitChange, len(commits)-1)

	for index, commit := range commits[1:] {
		lines := regexp.MustCompile(`\n`).Split(commit, -1)
		if len(lines) < 5 {
			continue
		}

		title := GetCommitTitle(lines)
		body := GetBody(lines)

		if !CommitIsValid(title) {
			change := GitChange{
				Body:           body,
			}
			gitChangesHolder[index] = change
			continue
		}

		committer := GetCommitter(lines)
		commitType := GetCommitType(title)
		breakingChangeFound := CommitHasBreakingChange(body)
		issueNum := GetIssueNum(body)
		issueLink := GetIssueLink(g.dir, issueNum)
		storyLink, _ := GetStoryLink(svc, owner, repo, issueNum)

		change := GitChange{
			Valid:          true,
			Title:          title,
			Body:           body,
			Issue:          issueNum,
			IssueLink: 		issueLink,
			StoryLink:      storyLink,
			Committer:      committer,
			CommitType:     commitType,
			BreakingChange: breakingChangeFound,
		}

		gitChangesHolder[index] = change

	}

	changes := GitChanges(gitChangesHolder)
	versionBump := changes.GetVersionBump()
	changeLog := GitChangeLog{
		VersionBump: versionBump,
		Changes:     changes,
		OutputMessage: GetChangeLogOutputMessage(versionBump, changes, options),
	}

	return changeLog, err
}

func CommitIsValid(title string) bool {
	valid, _ := regexp.MatchString(`.*(\()(.)*(\))(:){1}`, title)
	return valid
}

func CommitHasBreakingChange(body string) bool {
	var breakingChangeFound = true
	breakingChangeFormat := regexp.MustCompile(`BREAKING CHANGE: .*\n`)
	breakingChange := breakingChangeFormat.FindAllString(body, -1)

	if len(breakingChange) == 0 {
		breakingChangeFound = false
	}
	return breakingChangeFound
}

func (g GitChanges) GetVersionBump() string {
	majorChanges, minorChanges, patchChanges, _ := g.GetSeparatedChanges()

	if len(majorChanges) > 0 {
		return "MAJOR"
	} else if len(minorChanges) > 0 {
		return "MINOR"
	} else if len(patchChanges) > 0 {
		return "PATCHES"
	}
	return "NONE"
}

func (g GitChanges) GetSeparatedChanges() ([]GitChange, []GitChange, []GitChange, []GitChange) {
	majorChanges := make([]GitChange, 0)
	minorChanges := make([]GitChange, 0)
	patchChanges := make([]GitChange, 0)
	unknownChanges := make([]GitChange, 0)

	for _, change := range g {
		if change.BreakingChange {
			majorChanges = append(majorChanges, change)
		} else if _, ok := minorTypes[change.CommitType]; ok {
			minorChanges = append(minorChanges, change)
		} else if _, ok := patchTypes[change.CommitType]; ok {
			patchChanges = append(patchChanges, change)
		} else {
			unknownChanges = append(unknownChanges, change)
		}
	}

	return majorChanges, minorChanges, patchChanges, unknownChanges
}

func GetStoryLink(svc issues.IssueService, owner string, repo string, issue string) (string, error) {
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
	storyLink := new(strings.Builder)
	fmt.Fprintf(storyLink, "https://github.com/")
	fmt.Fprintf(storyLink, iss[1].Org)
	fmt.Fprintf(storyLink, "/")
	fmt.Fprintf(storyLink, iss[1].Repo)
	fmt.Fprintf(storyLink, "/issues/")
	fmt.Fprintf(storyLink, strconv.Itoa(iss[1].Number))

	return storyLink.String(), err
}

func GetIssueLink(dir string, issue string) string {
	linkFormat := regexp.MustCompile(`github\.com.*`)
	issueLink := new(strings.Builder)
	fmt.Fprintf(issueLink, "https://")
	fmt.Fprintf(issueLink, linkFormat.FindString(dir))
	fmt.Fprintf(issueLink, "/issues/")
	fmt.Fprintf(issueLink, issue)

	return issueLink.String()
}

func GetCommitTitle(lines []string) string {
	return lines[4]
}

func GetRepo(dir string) string {
	return strings.Split(dir, "/")[6]
}

func GetOwner(dir string) string {
	return strings.Split(dir, "/")[5]
}

func GetBody(lines []string) string {
	return strings.Join(lines[5:], "\n")
}

func GetCommitter(lines []string) string {
	return regexp.MustCompile(`Author: `).Split(lines[1], 2)[1]
}

func GetCommitType(title string) string {
	return strings.TrimSpace(regexp.MustCompile(`\(`).Split(title, 2)[0])
}

func GetIssueNum(body string) string {
	issueNoFormat := regexp.MustCompile(`([@#])[0-9]*`)
	issueNoMatches := issueNoFormat.FindAllString(body, -1)
	issue := issueNoMatches[len(issueNoMatches)-1]
	return issue[1:]
}

func GetChangeLogOutputMessage(bump string, changes GitChanges, options GitChangeLogOptions) string {
	majorChanges, minorChanges, patchChanges, unknownChanges := changes.GetSeparatedChanges()
	output := new(strings.Builder)
	fmt.Fprintf(output, "Recommended Version bump: %s\n\n", bump)
	fmt.Fprintf(output, "Major: %d\n", len(majorChanges))
	fmt.Fprintf(output, "Minor: %d\n", len(minorChanges))
	fmt.Fprintf(output, "Patch: %d\n", len(patchChanges))
	fmt.Fprintf(output, "Unknown: %d\n", len(unknownChanges))
	if len(majorChanges) != 0 {
		fmt.Fprintf(output, "\nMajor Changes: \n")
		for _, changes := range majorChanges {
			BuildOutput(output, changes, options)
		}
	}

	if len(minorChanges) != 0 {
		fmt.Fprintf(output, "\nMinor Changes: \n")
		for _, changes := range minorChanges {
			BuildOutput(output, changes, options)
		}
	}

	if len(patchChanges) != 0 {
		fmt.Fprintf(output, "\nPatch Changes: \n")
		for _, changes := range patchChanges {
			BuildOutput(output, changes, options)
		}
	}

	if len(unknownChanges) != 0 && options.Description {
		fmt.Fprintf(output, "\nUnknown Changes: \n")
		for _, changes := range unknownChanges {
			BuildUnknownOutput(output, changes)
		}
	}

	return output.String()
}

func BuildOutput(builder* strings.Builder, changes GitChange, options GitChangeLogOptions) {
	fmt.Fprintf(builder, "%s, by %s\n", changes.Title, changes.Committer)
	if options.Description {
		fmt.Fprintf(builder, "\t %s\n", changes.Body)
		if len(changes.StoryLink) > 0 {
			fmt.Fprintf(builder, "\t Issue: %s   Story: %s\n\n", changes.IssueLink, changes.StoryLink)
		}
	}

}

func BuildUnknownOutput(builder* strings.Builder, changes GitChange) {
	fmt.Fprintf(builder, "%s\n", changes.Body)
}