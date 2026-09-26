#!/usr/bin/env python3
"""Merge the legacy <vault>/agent task store into the shared vault.

Keeps an external full backup and an old-to-new ID map. Refuses ambiguous
metadata, duplicate IDs within either vault, and an already merged source.
"""
import argparse
import json
import re
import shutil
from datetime import datetime
from pathlib import Path

ID_RE = re.compile(r"^id:\s*(T-\d{8}-\d+)\s*$", re.M)
FIELD_RE = re.compile(r"^executor:\s*", re.M)


def views_block(path):
    text = path.read_text()
    match = re.search(r"^views:\n(?:[ \t].*\n|-[^\n]*\n)*", text, re.M)
    return match.group(0) if match else ""


def task_files(root):
    return sorted([*root.joinpath("tasks").rglob("*.md"), *root.joinpath("archive").rglob("*.md")])


def read_ids(files):
    found = {}
    for path in files:
        text = path.read_text()
        match = ID_RE.search(text)
        if not match:
            raise ValueError(f"missing ID: {path}")
        old_id = match.group(1)
        if old_id in found:
            raise ValueError(f"duplicate ID: {old_id}")
        found[old_id] = path
    return found


def merge(base, backup):
    source = base / "agent"
    if not (source / "tasks").is_dir():
        raise ValueError(f"legacy agent vault missing: {source}")
    if backup.exists():
        raise ValueError(f"backup already exists: {backup}")
    human = read_ids(task_files(base))
    agent = read_ids(task_files(source))
    if not agent:
        raise ValueError("no agent tasks to migrate")
    base_config, agent_config = base / "config.yaml", source / "config.yaml"
    base_views, agent_views = views_block(base_config), views_block(agent_config)
    if base_views and agent_views and base_views != agent_views:
        raise ValueError("saved views differ; merge config.yaml views before migration")
    for path in (source / "projects").rglob("project.md"):
        target = base / "projects" / path.relative_to(source / "projects")
        if target.exists() and target.read_bytes() != path.read_bytes():
            raise ValueError(f"project metadata conflict: {target}")
    next_seq = max([int(k.rsplit("-", 1)[1]) for k in [*human, *agent]], default=0) + 1
    mapping = {}
    used = set(human)
    for old in agent:
        if old in used:
            date = old.split("-")[1]
            new = f"T-{date}-{next_seq:04d}"
            next_seq += 1
        else:
            new = old
        if new in used or new in mapping.values():
            raise ValueError(f"new ID collision: {new}")
        mapping[old] = new
        used.add(new)
    staged = []
    for old, path in agent.items():
        text = path.read_text()
        if FIELD_RE.search(text):
            raise ValueError(f"already has executor field: {path}")
        for prior, current in mapping.items():
            text = re.sub(rf"(?<![\w-]){re.escape(prior)}(?![\w-])", current, text)
        text = text.replace("\nstatus:", "\nexecutor: agent\nstatus:", 1)
        if not FIELD_RE.search(text):
            raise ValueError(f"status field missing: {path}")
        relative = path.relative_to(source)
        target = base / relative.parent / path.name.replace(old, mapping[old], 1)
        if target.exists():
            raise ValueError(f"target exists: {target}")
        staged.append((target, text))
    backup.parent.mkdir(parents=True, exist_ok=True)
    shutil.copytree(base, backup)
    (backup / "id-map.json").write_text(json.dumps(mapping, ensure_ascii=False, indent=2) + "\n")
    try:
        for target, text in staged:
            target.parent.mkdir(parents=True, exist_ok=True)
            target.write_text(text)
        for path in (source / "projects").rglob("project.md"):
            target = base / "projects" / path.relative_to(source / "projects")
            if not target.exists():
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(path, target)
        if agent_views and not base_views:
            config_text = base_config.read_text()
            base_config.write_text(config_text.rstrip() + "\n" + agent_views)
        merged = read_ids(task_files(base))
        if len(merged) != len(human) + len(agent):
            raise ValueError("merged task count differs")
        if sum("executor: agent" in p.read_text() for p in merged.values()) < len(agent):
            raise ValueError("executor field verification failed")
    except Exception:
        for target, _ in staged:
            target.unlink(missing_ok=True)
        raise
    shutil.rmtree(source)
    print(json.dumps({"human": len(human), "agent": len(agent), "merged": len(merged), "remapped": sum(k != v for k, v in mapping.items()), "backup": str(backup)}, ensure_ascii=False))


if __name__ == "__main__":
    parser = argparse.ArgumentParser()
    parser.add_argument("--vault", type=Path, required=True)
    parser.add_argument("--backup", type=Path)
    args = parser.parse_args()
    base = args.vault.expanduser().resolve()
    backup = args.backup or base.parent / "tasks-migration-backups" / datetime.now().strftime("%Y%m%dT%H%M%S")
    merge(base, backup.expanduser().resolve())
