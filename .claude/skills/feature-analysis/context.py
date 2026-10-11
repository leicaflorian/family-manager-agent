#!/usr/bin/env python3
"""What /feature-analysis needs from the vault at every use, printed in one go: the ToDo items of the
agent with their state, and the rules (how a ToDo item is made, how notes are written), which are the
ones of Family Manager.

Run by SKILL.md while it loads (the !`…` line), so the skill starts with this already read and without a
round of file reads. It must never fail: a failed command stops the whole skill. On any problem it prints
what happened and ends with 0.

The path of the vault is not written here, it changes from one computer to another: it comes from $VAULT,
or from the folders of .claude/settings.local.json (permissions.additionalDirectories). Any folder inside
the vault will do: the root is the one with .obsidian.
"""
import json
import os
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[3]
AGENT = "Projects/Family Manager Agent"
MAIN = "Projects/Family Manager"
# the sections of the ToDo index of Family Manager with the rules; the tables of its items are not needed
TODO_RULES = ["Due note per ogni voce", "Intestazione delle note", "Quando una voce è fatta"]
WRITING_RULES = "Come sono scritte le note"


def vault_root():
    folders = [os.environ["VAULT"]] if os.environ.get("VAULT") else []
    settings = ROOT / ".claude/settings.local.json"
    if settings.is_file():
        folders += json.loads(settings.read_text(encoding="utf-8")).get("permissions", {}).get("additionalDirectories", [])
    for folder in folders:
        path = Path(folder).expanduser()
        for candidate in [path, *path.parents]:
            if (candidate / ".obsidian").is_dir():
                return candidate
    return None


def section(path, title):
    if not path.is_file():
        return f"## {title}\n\n(nota non trovata: {path})"
    found = re.search(rf"^## {re.escape(title)}\n.*?(?=^## |\Z)", path.read_text(encoding="utf-8"), re.S | re.M)
    return found.group(0).strip() if found else f"## {title}\n\n(sezione non trovata: leggere la nota)"


def prop(path, name):
    try:
        head = re.match(r"---\n(.*?)\n---", path.read_text(encoding="utf-8"), re.S)
        found = re.search(rf"^{re.escape(name)}:[ \t]*(.*)$", head.group(1), re.M) if head else None
        return found.group(1).strip().strip("\"'") if found else ""
    except OSError:
        return "?"


def items(folder):
    rows = []
    for item in sorted(p for p in folder.iterdir() if p.is_dir() and not p.name.startswith((".", "_"))):
        note = item / "Analisi.md"
        plan = "con piano" if (item / "Piano.md").exists() else "senza piano"
        rows.append(f"- {item.name} | {prop(note, 'tipo') or '?'} | {prop(note, 'stato') or '?'} | {plan}")
    return rows


def main():
    vault = vault_root()
    if vault is None:
        print("VAULT NON TROVATO: manca $VAULT, e in `.claude/settings.local.json` non c'è una cartella del vault "
              "(`permissions.additionalDirectories`, vedi `CLAUDE.md`).\n"
              "Fermarsi e dirlo all'utente: le note non si scrivono nel repository.")
        return
    print(f"Vault: {vault}")
    todo = vault / AGENT / "ToDo"
    print(f"\n### Voci del ToDo dell'agente, `{AGENT}/ToDo` (titolo | tipo | stato | piano)\n")
    if not todo.is_dir():
        print("(nessuna: la cartella `ToDo` non c'è ancora, nasce con la prima voce: vedi `analisi.md`, *Indice*)")
    else:
        print("\n".join(items(todo)) or "(nessuna)")
        done = todo / "_Done"
        if done.is_dir():
            names = sorted(p.name for p in done.iterdir() if p.is_dir() and not p.name.startswith("."))
            print("\n### Voci in `_Done` (fatte o scartate)\n\n" + "; ".join(names))
    print(f"\n### Regole delle voci (da `{MAIN}/ToDo/_index.md`: qui cambia solo la cartella, `{AGENT}/ToDo`)\n")
    print("\n\n".join(section(vault / MAIN / "ToDo/_index.md", title) for title in TODO_RULES))
    print(f"\n### Regole di scrittura (da `{MAIN}/_index.md`)\n")
    print(section(vault / MAIN / "_index.md", WRITING_RULES))
    print(f"\n### Cosa cambia per le note dell'agente (da `{AGENT}/_index.md`)\n")
    print(section(vault / AGENT / "_index.md", WRITING_RULES))


if __name__ == "__main__":
    try:
        main()
    except Exception as error:  # never stop the skill
        print(f"CONTESTO NON LETTO ({type(error).__name__}: {error}): leggere a mano `ToDo/_index.md` e `_index.md` "
              "di Family Manager nel vault.")
    sys.exit(0)
