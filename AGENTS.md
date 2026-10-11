# Family Manager Agent

Il programma in Go che comanda lo scanner attaccato al computer per conto della pagina *Scansiona* di Family Manager. Cosa fa, come si compila, come si prova e come si pubblica è nel `README.md`, da leggere per primo. Qui c'è quello che dal codice e dal README non si capisce. Vale per chiunque lavori qui, persona o agente.

## Dove sta il resto

- **Family Manager** (Directus con le sue estensioni) è un altro repository: `github.com/leicaflorian/family-manager`, in locale `~/Dev/family-manager-cms`. Lì ci sono la pagina che chiama l'agente (`extensions/document-reader/src/module/scan/`) e lo script che scarica gli zip pubblicati qui (`scripts/fetch-agent.mjs`). Ha le sue regole, in `CLAUDE.md`: prima di modificare qualcosa lì si leggono.
- **La documentazione** non sta in questo repository, a parte il `README.md`: è nel vault di Obsidian di Family Manager, su Google Drive.

  `/Users/florianleica/Library/CloudStorage/GoogleDrive-florian.leica@gmail.com/Il mio Drive/MyVault/Projects/Family Manager`

  | Nota | Cosa contiene |
  | --- | --- |
  | `02 - Documentazione tecnica/Agente dello scanner.md` | Com'è fatto l'agente e com'è fatta la pagina, cosa si è scoperto provando (con le date), i punti aperti |
  | `02 - Documentazione tecnica/How To/Compilare e installare l'agente dello scanner.md` | Compilare, pubblicare, installare, e cosa fare se qualcosa va storto |
  | `01 - Documentazione funzionale/Funzioni/Importazione dei documenti.md`, sezione *Dallo scanner di casa* | Cosa vede chi usa la pagina |
  | `03 - Branding/Branding.md`, sezione *Icona di Family Manager Agent* | Com'è fatta l'icona |

  Perché Claude Code legga il vault senza chiedere conferma, la cartella va aggiunta a `permissions.additionalDirectories` in `.claude/settings.local.json`: è il file personale, non versionato, e il percorso cambia da un computer all'altro.

## Regole

- **Il repository è pubblico.** Non ci entra niente che riguardi la famiglia: indirizzi di Family Manager, chiavi, token, scansioni, il contenuto di `origins.json` o di `agent.log`. L'agente non conosce Family Manager e deve restare così: il file lo carica la pagina, con il login di chi la usa.
- **Le scansioni di prova (`POST /scan`) si fanno solo quando il proprietario lo chiede**, con un foglio qualunque. Le immagini non si aprono, e alla fine si cancellano con `DELETE /pages`. Senza far passare fogli si prova con `./build.sh probe` e con `curl`.
- **Per una prova non si cambia `HOME`**: *Acquisizione Immagine* smette di vedere lo scanner. Per non far comparire la domanda "Permetti a…" si aggiunge il sito di prova a `origins.json`, prima di aprire l'agente.
- **Niente automazione del browser** (strumenti Chrome): costa troppi token. La pagina *Scansiona* la controlla il proprietario, con i passi da fare e cosa deve vedere.
- **Il numero di versione lo sceglie il proprietario.** Si propone e si chiede.
- **Si pubblica solo quando il proprietario lo chiede**, con `./release.sh` (o la skill `/release`): fa il push e crea una release pubblica, e una versione pubblicata non si pubblica di nuovo.
- **Le memorie si scrivono anche nel progetto.** Ogni regola o informazione che un agente salva nella propria memoria va scritta anche qui, in `CLAUDE.md` e `AGENTS.md`, o nella nota `Agente dello scanner` del vault se è una cosa scoperta lavorando sul codice: la memoria di un agente gli altri non la leggono. I due file restano identici.

## Lavorare sul codice

- **Prima di modificare il codice si legge la nota `Agente dello scanner`** del vault: le cose scoperte provando lo scanner (l'ordine delle conferme di *Acquisizione Immagine*, i tempi di attesa, i codici di WIA) stanno lì, con le date. A lavoro finito la nota si aggiorna, con la data.
- **Quello che lega l'agente alla pagina** sono la porta `47115` e le richieste (tabella del README). La porta è scritta in tre posti, da tenere uguali: `server.go` (`Port`), `scan/agent.ts` di Family Manager (`AGENT_URL`) e la variabile `CONTENT_SECURITY_POLICY_DIRECTIVES__CONNECT_SRC` di Directus (in `docker-compose.yml` e su Railway). Chi cambia una richiesta qui la cambia anche nella pagina, e lo dice: le due cose escono con due rilasci diversi, e chi ha già l'agente vecchio continua a usarlo finché non scarica quello nuovo.
- **La parte di Windows è scritta senza poterla provare**: dal Mac si sa solo che si compila. Di una modifica a `scanner_windows.go`, `install_windows.go` o `dialog_windows.go` si dice che è compilata, non che funziona. Sul PC la prova il proprietario, con `scanner-probe.exe` e `test-windows.cmd`.
- **Il controllo è `./build.sh`**: `go vet` per i due sistemi e la compilazione di tutti e due i programmi. Non ci sono test automatici. Un `go build` semplice controlla solo il Mac.
- **I commenti del codice e i messaggi dei commit sono in inglese**; quello che l'utente legge (domande, messaggi, voci del menu), il README e le note sono in italiano.
- **L'icona sta in due repository**: `icon.png` qui e `branding/family-manager-agent-icon-512.png` in Family Manager. Cambiando il disegno si sostituiscono tutti e due.

## Documentazione

- "Aggiornare la documentazione" vuol dire il vault **e** il `README.md`, non solo il README.
- **Prima di un commit la documentazione del codice committato dev'essere aggiornata**: si guarda cosa c'è nel commit e si controlla che il README e le note del vault dicano quello che il codice fa adesso. Vale anche quando la richiesta è solo "fai il commit".
- Le regole su come sono scritte le note stanno nel vault e si rileggono prima di scrivere: `_index.md`, sezione *Come sono scritte le note* (niente titolo `#`, collegamenti alle note vicine nelle proprietà, una nota `_index` per cartella). Una nota che esiste si modifica per sostituzioni, e si rilegge subito prima: nel vault lavorano anche altri agenti e il proprietario.
- Il controllo finale delle note è in Family Manager e si lancia da qualunque cartella: `python3 ~/Dev/family-manager-cms/scripts/vault-check.py`. Segnala le note fuori regola senza correggerle.
- Le cose da fare (una funzione nuova, un'idea) sono voci del `ToDo/` del vault e si scrivono da Family Manager, con la sua skill `/feature-analysis`.

## Versioni e rilasci

- Si sviluppa sul branch `dev`, come in Family Manager (dall'11/10/2026): i commit vanno lì. `main` contiene solo le versioni pubblicate e non ci si committa: per pubblicare si porta `dev` in `main` (`git merge --ff-only dev`) e da `main` si lancia `release.sh`, che parte solo da lì e con tutto committato. Poi si torna su `dev`.
- Prima di un commit si controllano `git status` e il branch (`git branch --show-current`), e non si toccano i file non committati che non sono propri.
- Il push rende pubblico il commit: si fa solo quando il proprietario lo chiede, o con `release.sh`.
- La versione è nel file `VERSION`. Va alzata a ogni pubblicazione: la pagina avvisa chi ha un agente più vecchio di quello che Family Manager offre.
- **Pubblicare qui non basta.** Family Manager offre la versione scritta in `scripts/fetch-agent.mjs` (`VERSION` e le tre impronte, che `release.sh` stampa alla fine): finché lo script non cambia e Family Manager non viene rilasciato, la pagina continua a offrire quella di prima.
