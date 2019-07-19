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
	IsSquashMerge  bool
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

const Major = "major"
const Minor = "minor"
const Patch = "patch"
const Unknown = "unknown"

type GitChanges []GitChange
var allTypes = []string{"feat","perf","test","chore","fix","docs","style","refactor"}
var bumpMap = map[string]string{"fix": Patch, "docs": Patch, "style": Patch, "refactor": Patch,
								"feat": Minor, "perf": Minor, "test": Minor, "chore": Minor}

func (g GitWrapper) ChangeLog(logPath string, command string, svc issues.IssueService, options GitChangeLogOptions) (GitChangeLog, error) {
	args := append([]string{"-C", g.dir, "log"}, logPath, command)
	out, err := pkg.NewCommand("git", args...).RunOut()
	commits := regexp.MustCompile(`commit .{40}`).Split(out, -1)
	owner, repo := GetOrgAndRepoFromPath(g.dir)
	gitChangesHolder := make([]GitChange, len(commits)-1)
	squashMergeChangesHolder := make([]GitChange, 0)

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

		if isSquashMerge(body) {

			squashMergeChangesHolder =  append(squashMergeChangesHolder, GetSquashMergeChanges(&change)...)
		}

		gitChangesHolder[index] = change

	}
	gitChangesHolder = append(gitChangesHolder, squashMergeChangesHolder...)
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
	return regexp.MustCompile(strings.Join(allTypes, "|")).MatchString(title)
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
	fmt.Fprintf(&builder, "https://github.com/%s/%s/issues/%s",iss[1].Org,iss[1].Repo,strconv.Itoa(iss[1].Number))

	return builder.String(), err
}

func GetIssueLink(dir string, issue string) string {
	linkFormat := regexp.MustCompile(`github\.com.*`)
	var builder = strings.Builder{}
	fmt.Fprintf(&builder, "https://%s/issues/%s", linkFormat.FindString(dir), issue)
	return builder.String()
}

func GetCommitTitle(lines []string) string {
	return lines[4]
}

func GetBody(lines []string) string {
	return strings.Join(lines[5:], "\n")
}

func GetCommitter(lines []string) string {
	return regexp.MustCompile(`Author: `).Split(lines[1], 2)[1]
}

func isSquashMerge(body string) bool {
	titleFormat :=  regexp.MustCompile(`.*\(.*\): .*\n`)
	titleMatches := titleFormat.FindAllString(body, -1)
	if len(titleMatches) > 0 {
		return true
	}
	return false
}

func GetCommitType(title string) string {
	return regexp.MustCompile(strings.Join(allTypes, "|")).FindString(title)
}

func GetIssueNum(body string) string {
	issueNoFormat := regexp.MustCompile(`[0-9]+`)
	issueNoMatches := issueNoFormat.FindAllString(body, -1)
	if len(issueNoMatches) == 0{
		return ""
	}
	issue := issueNoMatches[len(issueNoMatches)-1]
	return issue
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
	fmt.Fprintf(&builder, "%s, by %s\n", changes.Title, changes.Committer)
	if options.Description {
		fmt.Fprintf(&builder, "\t %s", changes.Body)
		if len(changes.StoryLink) > 0 {
			fmt.Fprintf(&builder, "\n\t Issue: %s   Story: %s\n", changes.IssueLink, changes.StoryLink)
		}
		if changes.IsSquashMerge {
			fmt.Fprintf(&builder, "\t Squash merge\n")
		}
	}
	return strings.Replace(builder.String(), "\n\n", "\n", -1)
}

func BuildUnknownOutput(builder* strings.Builder, changes GitChange) {
	fmt.Fprintf(builder, "%s\n", changes.Body)
}

func GetSquashMergeChanges(change* GitChange) []GitChange {
	squashMergeChanges := make([]GitChange, 0)
	bodyArr := strings.Split(change.Body,"\n")
	issueLine := bodyArr[len(bodyArr) - 1]
	var title = ""
	var builder = strings.Builder{}
	for _, line := range bodyArr[1:] {
		if regexp.MustCompile(`.*\(.*\): .*`).MatchString(line) {
			if title != "" {
				fmt.Fprintf(&builder, "%s\n", issueLine)
				squashChange := GitChange{
					Valid:          true,
					Title:          title,
					Body:           builder.String(),
					Issue:          change.Issue,
					IssueLink: 		change.IssueLink,
					StoryLink:      change.StoryLink,
					Committer:      change.Committer,
					CommitType:     GetCommitType(title),
					BreakingChange: CommitHasBreakingChange(builder.String()),
					IsSquashMerge: true,
				}

				squashMergeChanges = append(squashMergeChanges, squashChange)
				builder = strings.Builder{}
			}
			title = line
			continue
		}
		fmt.Fprintf(&builder, "%s\n", line)
	}

	if title != "" {
		squashChange := GitChange{
			Valid:          true,
			Title:          title,
			Body:           builder.String(),
			Issue:          change.Issue,
			IssueLink: 		change.IssueLink,
			StoryLink:      change.StoryLink,
			Committer:      change.Committer,
			CommitType:     GetCommitType(title),
			BreakingChange: CommitHasBreakingChange(builder.String()),
			IsSquashMerge: true,
		}
		squashMergeChanges = append(squashMergeChanges, squashChange)
	}
	builder = strings.Builder{}
	fmt.Fprintf(&builder, "%s\n%s", bodyArr[0], issueLine)
	change.Body = builder.String()
	return squashMergeChanges
}
