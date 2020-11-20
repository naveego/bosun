# How to create, deploy, and commit a release

The release commands will work from any location, but they all operate on the current platform
and manipulate files under `platform/releases` and `platform/deploys` in devops.


## Folders

On release branch:
`releases/stable` is the current release you're working on, comprising the files from the release branches of 
the apps you're updating and the files from the previous release for apps you are not updating.
`releases/unstable` comprises the latest versions of every app, from their develop branches as of when the release 
was created. 

On non-release branch:
`/releases/stable` is the most recent release; bosun will not make changes to this unless you're on a release branch
`/releases/unstable` comprises the latest versions of every app, from their develop branches, updated manually by 
running bosun release update unstable --all. This exists to support doing a deploy without having all the repos cloned.

> Important:
> Most `bosun release` commands use `git worktree` (https://git-scm.com/docs/git-worktree) when interacting with
> repos outside of devops, to avoid interfering with branches you have checked out. This can lead to unexpected
> results if you have changes in some you want bosun to use. It's a good policy to always commit any changes
> you want bosun to pick up. 

## Plan

The first step is creating a release plan, which is a document which helps decide what will be in the release.
To plan a release, check out the branch you want to base the release on (usually `develop`) and run
`bosun release plan start --name $NAME --version $VERSION`. This will create a release branch like `release/$VERSION`
and check it out, and will create a plan file in `releases/stable`. It will also update every app in 
`releases/unstable` by copying files from the `develop` branch of the repo for the app. Finally, it will create
a `releases/stable/plan.yaml` file which you can use to determine which apps will be upgraded in the release.

Once you have your `plan.yaml` you can open it in code and edit it. For each app which has changes you need to choose
what provider (stable or unstable) you want to get the files for that app from. If you choose `stable` then
nothing will be changed, it will use the existing files (the files from the previous release). If you choose 
`unstable` the files will be copied from the `unstable` release (which was just updated from the `develop` branches).
If you choose `unstable` you can also specify the `bump` to apply to change the version number. Eventually all
apps will be auto-bumped during release commit, but right now there are some that are not, so you may want to specify
the bump manually. You can also specify whether you want an app to be deployed in the release, which you usually do
if you choose `unstable`.

After you've selected a provider for each app that had changes, you can run `bosun release plan commit`. This will
update the `manifest.yaml` for the release with all the apps you've chosen. It will also create release branches
for all the apps you've chosen to upgrade in this release. 

## Test and Refine

You can now create a deploy from the release using `bosun deploy plan release`. This will create a deployment in
`platform/deployments` with the name `$VERSION`. You can deploy that to uat or preprod to test out the release using
`bosun deploy execute`. If `docker` requires sudo you can use `bosun deploy execute --sudo` to allow the image 
validation logic to run, or `bosun deploy execute --skip-validation` to just skip validation if you already know
the images exist. You can use `bosun deploy execute [apps...]` to deploy specific apps from the release. There is also 
a `--cluster` flag to limit what clusters get deployed to.

If you make changes to an app that you want to include in the release:
1. Make sure the changes are *committed* on the release branch. 
2. Run `bosun release update stable $APP` to update the app manifests in the release
3. Run `bosun deploy plan release` to update the deployment. 
4. Run `bosun deploy execute $APP` to re-deploy your app.

If you want to add an app to the release, run `bosun release add stable [apps]` for step 2. There are `--branch` and 
`--bump` flags to adjust how and from what branch the app is added.

## Commit
After the release is deployed to production, you need to merge everything back.

1. Run `bosun release commit plan` on the release branch to create a commit plan. This will create a file 
in `/tmp` and log the name. You can edit the file or run `bosun release commit show` to see what's in it.
The file contains all the steps which will be taken to commit the release. It tracks progress and errors
because sometimes you run into merge conflicts which are complicated and it takes a few tries to finish the commit.
2. Run `bosun release commit execute` to execute the commit. If you run into issues, fix them, then run the command 
again. It will skip anything already completed. If you want to run a step again, edit the plan file and delete
the `completedAt` element for the step you want to repeat.  