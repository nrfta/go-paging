import argparse
import json
import os
import requests

PROJECT_GID = "1202267217415053"
SECTION_GID = "1203075160692525"
ASANA_API_TOKEN = os.getenv("ASANA_API_TOKEN")
GITHUB_TOKEN = os.getenv("GITHUB_TOKEN")


def post_to_asana(uri, data, method="POST"):
    headers = {
        "Authorization": f"Bearer {ASANA_API_TOKEN}",
        "Content-Type": "application/json",
        "Accept": "application/json",
    }
    res = requests.request(
        method=method,
        url=f"https://app.asana.com/api/1.0{uri}",
        json=data,
        headers=headers,
    )
    if res.status_code > 299:
        raise Exception(res.text)

    return res.json()


def delete_cm_task(cm_task_info):
    post_to_asana(f"/tasks/{cm_task_info['task_gid']}", None, method="DELETE")


def post_to_github(uri, data, method="POST"):
    headers = {
        "Accept": "application/vnd.github+json",
        "Authorization": f"Bearer {GITHUB_TOKEN}",
    }
    res = requests.request(
        method=method, url=f"https://api.github.com{uri}", json=data, headers=headers
    )
    if res.status_code > 299:
        raise Exception(res.text)

    return res.json()


def add_comment_to_cm_task(cm_task_info, comment):
    post_data = {"data": {"text": comment}}
    post_to_asana(f"/tasks/{cm_task_info['task_gid']}/stories", post_data)


def create_cm_task(args, pr_data):
    repo_name = args.repo.split("/")[1]
    task_name = f"{repo_name}: {pr_data['title']}"
    post_data = {
        "data": {
            "projects": [PROJECT_GID],
            "followers": [],
            "name": task_name,
        }
    }

    task = post_to_asana("/tasks", post_data)
    task_data = task["data"]
    cm_task_info = {
        "task_gid": task_data["gid"],
        "task_url": task_data["permalink_url"],
    }

    # add PR URL as comment on the task
    comment = (
        f"PR created by {pr_data['user']['login']}: {pr_data['_links']['html']['href']}"
    )
    add_comment_to_cm_task(cm_task_info, comment)

    # Add task to PR section
    post_data = {"data": {"task": task_data["gid"]}}
    post_to_asana(f"/sections/{SECTION_GID}/addTask", post_data)

    return cm_task_info


def add_cm_details_to_pr_body(pr_body, cm_task_url):
    return f"""{pr_body}

<details>
<summary>Change Management</summary>
<a href="{cm_task_url}">Asana task</a>
</details>
"""


def remove_cm_details_from_pr_body(pr_body):
    new_body = ""
    in_cm = False
    test_in_cm = False
    for line in pr_body.splitlines():
        if not test_in_cm and line == "<details>":
            test_in_cm = True
            continue

        if test_in_cm:
            if line == "<summary>Change Management</summary>":
                in_cm = True
                test_in_cm = False
            else:
                new_body = new_body + "\n<details>"

        if not in_cm:
            new_body = new_body + f"\n{line}"

        if in_cm and line == "</details>":
            in_cm = False

    return new_body


def update_pr_description(args, pr_data, cm_task_info):
    post_data = {
        "body": add_cm_details_to_pr_body(pr_data["body"], cm_task_info["task_url"])
    }
    post_to_github(f"/repos/{args.repo}/pulls/{pr_data['number']}", post_data)


def parse_cm_info_from_pr_body(pr_body):
    next_line = False
    for line in pr_body.splitlines():
        if line == "<summary>Change Management</summary>":
            next_line = True
        elif next_line:
            href_index = line.index("href")
            url_start_index = line.index('"', href_index) + 1
            url_end_index = line.index('"', url_start_index)
            url = line[url_start_index:url_end_index]
            url_parts = url.split("/")
            gid = url_parts[len(url_parts) - 1]
            return {"task_gid": gid, "task_url": url}
    return None


def update_cm_task_name(args, pr_data, cm_task_info):
    repo_name = args.repo.split("/")[1]
    task_name = f"{repo_name}: {pr_data['title']}"
    post_data = {"data": {"name": task_name}}
    post_to_asana(f"/tasks/{cm_task_info['task_gid']}", post_data, method="PUT")


def complete_cm_task(cm_task_info):
    post_data = {"data": {"completed": True}}
    post_to_asana(f"/tasks/{cm_task_info['task_gid']}", post_data, method="PUT")


def remove_cm_from_pr_description(args, pr_data):
    post_data = {"body": remove_cm_details_from_pr_body(pr_data["body"])}
    post_to_github(f"/repos/{args.repo}/pulls/{pr_data['number']}", post_data)


if __name__ == "__main__":
    parser = argparse.ArgumentParser("Change management for GitHub pull requests")
    parser.add_argument(
        "repo",
        help="github repo in org/repo format",
    )
    parser.add_argument(
        "pr_info_file",
        help="file containing the PR info in JSON format",
    )
    parser.add_argument(
        "github_event_action",
        help="action that is occuring in GitHub",
    )

    args = parser.parse_args()
    print("repo: " + args.repo)
    print("pr_info_file: " + args.pr_info_file)
    print("github_event_action: " + args.github_event_action)

    with open(args.pr_info_file) as f:
        pr_data = json.load(f)

    if pr_data["body"] is None:
        pr_data["body"] = ""
        
    if "<summary>Change Management</summary>" not in pr_data["body"]:
        cm_task_info = create_cm_task(args, pr_data)
        update_pr_description(args, pr_data, cm_task_info)
    else:
        cm_task_info = parse_cm_info_from_pr_body(pr_data["body"])

    print(cm_task_info)

    if args.github_event_action == "edited":
        update_cm_task_name(args, pr_data, cm_task_info)

    if args.github_event_action == "closed":
        if pr_data["merged"]:
            comment = f"PR merged by {pr_data['user']['login']}"
            add_comment_to_cm_task(cm_task_info, comment)
            complete_cm_task(cm_task_info)
        else:
            remove_cm_from_pr_description(args, pr_data)
            delete_cm_task(cm_task_info)
