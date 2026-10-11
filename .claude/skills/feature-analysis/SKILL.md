---
name: feature-analysis
description: Scrive nel vault l'analisi di una voce del ToDo di Family Manager Agent e, dopo le risposte del proprietario, il piano d'azione. Da usare per analizzare una funzione o un'idea dell'agente, o per scriverne il piano. Non scrive codice.
argument-hint: "[titolo della voce o descrizione della funzione]"
allowed-tools: Bash(python3 ${CLAUDE_SKILL_DIR}/context.py)
---

# Analisi di una funzione e piano d'azione

Una voce del ToDo si lavora in **due note** del vault, una dopo l'altra: prima l'**analisi**, poi il **piano d'azione**. In mezzo ci sono le risposte del proprietario ai punti da decidere. Questa skill scrive le due note; lo sviluppo non ne fa parte e parte solo quando il proprietario lo chiede.

Il vault è quello di Obsidian del proprietario, e qui servono tre sue cartelle: `Projects/Family Manager Agent` (le note dell'agente e il suo `ToDo`), `Projects/Family Manager` (le regole, e le note della pagina *Scansiona*) e `Template` (i modelli). Il percorso sul computer cambia da un computer all'altro: `context.py` lo trova dalle cartelle di `.claude/settings.local.json` (`CLAUDE.md`, *Dove sta il resto*), ed è stampato qui sotto in *Dal vault, adesso*. Se il vault non si legge ci si ferma e lo si dice: le note non si scrivono nel repository.

Questa skill è divisa in più file per leggere solo quello che serve: questo, `analisi.md` e `piano.md`, nella stessa cartella (`.claude/skills/feature-analysis/`). Dei due si legge **solo quello della fase** in cui è la voce. `indice.md` serve una volta sola, alla prima voce.

Accanto c'è `context.py`: viene eseguito mentre questo file si carica, e quello che stampa è già qui sotto, in *Dal vault, adesso*.

## Leggere e scrivere poco

Quello che si legge resta nella conversazione fino alla fine e si paga a ogni passo; quello che si scrive costa ancora di più. Valgono per tutte e due le fasi:

- **Le regole sono già qui sotto**, in *Dal vault, adesso*: come è fatta una voce e come sono scritte le note. Sono quelle di Family Manager; qui non sono copiate, e i due indici non si rileggono.
- **Le letture si fanno insieme, in un solo passo**: il file della fase, il modello e la nota della voce nello stesso momento, non uno dopo l'altro. Lo stesso per le note della raccolta.
- **Una nota lunga non si legge intera**: prima i titoli (`grep -n '^#'`), poi la sezione che serve. Una nota già letta in questa conversazione non si rilegge.
- **Una nota che esiste si modifica, non si riscrive.** Per aggiornare un'analisi, scrivere le risposte, cambiare `stato` o una riga dell'indice si sostituisce solo il pezzo che cambia. Si scrive una nota intera solo quando nasce (o quando un'idea prende la struttura del modello).
- **Una fase per conversazione.** Il piano non ha bisogno della conversazione in cui è nata l'analisi: tutto quello che serve è nella nota. Finita l'analisi si dice all'utente che per il piano conviene una conversazione nuova (`/clear`, poi `/feature-analysis <Titolo>`).

## Quale voce e a che punto è

**Prima: è una voce dell'agente?** Qui si scrivono le voci che cambiano il programma (cosa fa sul computer, lo scanner, le richieste, come si installa e si aggiorna). Una voce che cambia solo la pagina *Scansiona* o un'altra parte di Family Manager è del suo ToDo (`Projects/Family Manager/ToDo`) e si scrive da quel repository, con la sua `/feature-analysis`: lo si dice e ci si ferma. Una voce che tocca tutti e due sta dove c'è più lavoro, e dice nell'analisi cosa cambia dall'altra parte.

Cercare la voce nell'elenco qui sotto (*Voci del ToDo dell'agente*, con `tipo`, `stato` e se ha già il piano), dal titolo o dalla descrizione ricevuta. Senza argomento: mostrare le voci con `tipo: idea` e chiedere quale.

| Cosa si trova | Cosa si fa |
| --- | --- |
| Nessuna cartella | Si crea la cartella `<Titolo>/` e dentro l'analisi: leggere `analisi.md` |
| `Analisi` con `tipo: idea` | L'analisi si scrive nella **stessa nota**: leggere `analisi.md` |
| `Analisi` con `tipo: analisi` e `piano` vuoto | Piano d'azione, `Piano.md` nella stessa cartella: leggere `piano.md` |
| `Analisi` con `piano` compilato, oppure voce in `_Done` | Dire a che punto è la voce e non riscrivere niente; si aggiorna solo quello che l'utente chiede |

Il titolo di una voce nuova è breve e dice cosa cambia per chi usa l'agente, senza `/`, `\` e `:`. Si sceglie e si va avanti: l'utente lo può cambiare dopo.

## Note

- Si scrive solo nelle note di questa voce e in `ToDo/_index.md` dell'agente. Le note di `02 - Documentazione tecnica` descrivono quello che l'agente fa oggi: si aggiornano a sviluppo fatto (`CLAUDE.md`, sezione *Documentazione*), non qui.
- Non si spostano voci in `_Done` e non si aggiunge il callout `[!success]`: succede a lavoro finito (regola qui sotto, *Quando una voce è fatta*).
- Nel vault lavorano anche altri agenti e il proprietario: una nota va riletta subito prima di modificarla, e nell'indice si tocca solo la riga della propria voce.
- Se l'analisi mostra che la voce non conviene (costo, una cosa che il sistema non permette, già coperta da un'altra voce), lo si scrive nella *Proposta* e lo si mette come primo punto di *Da decidere*: scartarla è una scelta del proprietario.

## Dal vault, adesso

Stampato da `context.py` al caricamento di questa skill. Se qui sotto non c'è l'elenco delle voci (comando non eseguito), si legge a mano: `ls` di `Projects/Family Manager Agent/ToDo`, `Projects/Family Manager/ToDo/_index.md` fino alla sezione *Quando una voce è fatta* compresa, e la sezione *Come sono scritte le note* dei due `_index.md` (`Projects/Family Manager` e `Projects/Family Manager Agent`).

!`python3 ${CLAUDE_SKILL_DIR}/context.py`
