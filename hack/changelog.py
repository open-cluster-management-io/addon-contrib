import sys
import os

from enum import Enum
from github import Github
from github import Auth

emojiFeature = str('✨')
emojiBugfix = str('🐛')
emojiDocs = str('📖')
emojiInfra = str('🌱')
emojiBreaking = str('⚠')


class PRType(Enum):
    UncategorizedPR = 1
    BreakingPR = 2
    FeaturePR = 3
    BugfixPR = 4
    DocsPR = 5
    InfraPR = 6


def pr_type_from_title(pr_title: str):
    t = pr_title.strip()
    if len(pr_title) == 0:
        return PRType.UncategorizedPR, t

    if pr_title.startswith(':sparkles:') or pr_title.startswith(emojiFeature):
        return PRType.FeaturePR, t.removeprefix(':sparkles:').removeprefix(emojiFeature).strip()
    elif pr_title.startswith(':bug:') or pr_title.startswith(emojiBugfix):
        return PRType.BugfixPR, t.removeprefix(':bug:').removeprefix(emojiBugfix).strip()
    elif pr_title.startswith(':book:') or pr_title.startswith(emojiDocs):
        return PRType.DocsPR, t.removeprefix(':book:').removeprefix(emojiDocs).strip()
    elif pr_title.startswith(':seedling:') or pr_title.startswith(emojiInfra):
        return PRType.InfraPR, t.removeprefix(':seedling:').removeprefix(emojiInfra).strip()
    elif pr_title.startswith(':warning:') or pr_title.startswith(emojiBreaking):
        return PRType.BreakingPR, t.removeprefix(':warning:').removeprefix(emojiBreaking).strip()

    return PRType.UncategorizedPR, t


def change_entry(pr_title, number, author):
    return "%s (#%d) @%s" % (pr_title, number, author)


def section_if_present(changes: [], pr_title):
    if len(changes) > 0:
        print("")
        print("## %s\n" % pr_title)
        print("")
        for change in changes:
            print("- %s\n" % change)


if __name__ == '__main__':
    args = sys.argv[1:]
    token = args[0]
    if len(token) == 0:
        token = os.getenv("GITHUB_TOKEN")
    auth = Auth.Token(token)
    repo_name = args[1]
    release_tag = args[2]
    g = Github(auth=auth)
    repo = g.get_repo("open-cluster-management-io/addon-contrib")
    pulls = repo.get_pulls(state='closed', sort='created', base='main', direction='desc')

    # get the last release tag for this specific sub repo
    tags = repo.get_tags()
    if tags.totalCount == 0:
        print("no tags in the repo")
        sys.exit()

    # Filter tags that match the current repo_name prefix
    # Use exact prefix match to avoid matching similar repo names
    repo_tags = []
    for tag in tags:
        if tag.name.startswith(repo_name + '/'):
            repo_tags.append(tag)

    if len(repo_tags) <= 1:
        # First release for this repo - no previous tags to compare
        # (0 = not tagged yet, 1 = only current tag exists)
        last_release_tag = None
    else:
        # Find the previous release tag (second in the filtered list)
        # repo_tags[0] is current release, repo_tags[1] is previous
        last_release_tag = repo_tags[1].name

    # get related PR from the last release tag
    last_release_pr = 0
    release_word = "in"

    if last_release_tag:
        release_word = "since"
        # Find the tag object to get its commit and associated PRs
        for tag in tags:
            if tag.name == last_release_tag:
                tag_pulls = tag.commit.get_pulls()
                last_release_pr = tag_pulls[0].number
                break

    # collect all PRs from the last release tag
    features = []
    bugs = []
    breakings = []
    docs = []
    infras = []
    uncategorized = []
    for pr in pulls:
        if pr.number == last_release_pr:
            break
        if pr.is_merged():
            if repo_name not in [l.name for l in pr.labels]:
                continue
            prtype, title = pr_type_from_title(pr.title)
            if prtype == PRType.FeaturePR:
                features.append(change_entry(title, pr.number, pr.user.login))
            elif prtype == PRType.BugfixPR:
                bugs.append(change_entry(title, pr.number, pr.user.login))
            elif prtype == PRType.DocsPR:
                docs.append(change_entry(title, pr.number, pr.user.login))
            elif prtype == PRType.InfraPR:
                infras.append(change_entry(title, pr.number, pr.user.login))
            elif prtype == PRType.BreakingPR:
                breakings.append(change_entry(title, pr.number, pr.user.login))
            elif prtype == PRType.UncategorizedPR:
                uncategorized.append(change_entry(title, pr.number, pr.user.login))

    # Print
    print("# %s %s" % (repo_name, release_tag))
    if last_release_tag:
        print("\n**changes %s [%s](https://github.com/open-cluster-management-io/releases/%s)**\n"
              % (release_word, last_release_tag, last_release_tag))
    else:
        print("\n**Initial release**\n")
    section_if_present(breakings, ":warning: Breaking Changes")
    section_if_present(features, ":sparkles: New Features")
    section_if_present(bugs, ":bug: Bug Fixes")
    section_if_present(docs, ":book: Documentation")
    section_if_present(infras, ":seedling: Infra & Such")
    section_if_present(uncategorized, ":hammer_and_wrench: Uncategorized")
    print("")
    print("Thanks to all our contributors!*")
