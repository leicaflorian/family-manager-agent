# Family Manager Agent

Il programma in Go che comanda lo scanner attaccato al computer per conto della pagina *Scansiona* di Family Manager. Cosa fa, come si compila, come si prova e come si pubblica è nel `README.md`, da leggere per primo. Qui c'è quello che dal codice e dal README non si capisce. Vale per chiunque lavori qui, persona o agente.

## Dove sta il resto

- **Family Manager** (Directus con le sue estensioni) è un altro repository: `github.com/leicaflorian/family-manager`, in locale `~/Dev/family-manager-cms`. Lì ci sono la pagina che chiama l'agente (`extensions/document-reader/src/module/scan/`) e lo script che scarica gli zip pubblicati qui (`scripts/fetch-agent.mjs`). Ha le sue regole, in `CLAUDE.md`: prima di modificare qualcosa lì si leggono.
- **La documentazione** non sta in questo repository, a parte il `README.md` e il `CHANGELOG.md`: è nel vault di Obsidian, su Google Drive, in una cartella sua (dall'11/10/2026; prima le note stavano tra quelle di Family Manager).

  `/Users/florianleica/Library/CloudStorage/GoogleDrive-florian.leica@gmail.com/Il mio Drive/MyVault/Projects/Family Manager Agent`

  | Nota | Cosa contiene |
  | --- | --- |
  | `_index.md` | Da dove si parte, e cosa è rimasto tra le note di Family Manager |
  | `02 - Documentazione tecnica/Agente dello scanner.md` | Com'è fatto l'agente e com'è fatta la pagina, cosa si è scoperto provando (con le date), i punti aperti |
  | `02 - Documentazione tecnica/Versioni dell'agente.md` | Le versioni pubblicate, come si numerano, i due branch |
  | `02 - Documentazione tecnica/How To/Compilare e installare l'agente dello scanner.md` | Compilare, pubblicare, installare, e cosa fare se qualcosa va storto |

  Quello che riguarda la pagina è rimasto tra le note di Family Manager, nella cartella accanto (`/Users/florianleica/Library/CloudStorage/GoogleDrive-florian.leica@gmail.com/Il mio Drive/MyVault/Projects/Family Manager`):

  | Nota | Cosa contiene |
  | --- | --- |
  | `01 - Documentazione funzionale/Funzioni/Importazione dei documenti.md`, sezione *Dallo scanner di casa* | Cosa vede chi usa la pagina |
  | `03 - Branding/Branding.md`, sezione *Icona di Family Manager Agent* | Com'è fatta l'icona |

  Perché Claude Code legga il vault senza chiedere conferma, le due cartelle vanno aggiunte a `permissions.additionalDirectories` in `.claude/settings.local.json`: è il file personale, non versionato, e il percorso cambia da un computer all'altro.

## Regole

- **Il repository è pubblico.** Non ci entra niente che riguardi la famiglia: indirizzi di Family Manager, chiavi, token, scansioni, il contenuto di `origins.json` o di `agent.log`. L'agente non conosce Family Manager e deve restare così: il file lo carica la pagina, con il login di chi la usa. L'unico indirizzo che l'agente chiama è quello delle sue release su GitHub, per aggiornarsi (`updateSource` in `update.go`): non se ne aggiungono altri, e in quella richiesta non va niente del computer o di chi lo usa.
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
- **Il controllo è `./build.sh`**: `go vet` per i due sistemi, i test, e la compilazione di tutti e due i programmi. Un `go build` semplice controlla solo il Mac.
- **I test sono i file `_test.go`, accanto al codice** (dall'11/10/2026), nel pacchetto `main`: non c'è una cartella `src` né una dei test. Coprono `origins.go`, `server.go`, `scanner.go` e `update.go`, con la sostituzione dell'app sul Mac; non le parti che parlano con lo scanner vero (`scanner_darwin.go`, `scanner_windows.go`), le finestre e il menu. Da soli girano con `go test ./...`. Girano solo sul Mac: di un test per Windows si dice che è compilato. Un test non apre l'agente, non tocca lo scanner, non chiama la rete e non scrive nelle cartelle dell'agente installato (`origins.json`, `agent.log`, la cache): usa `t.TempDir()`, uno scanner finto (`feeder`), delle release finte (`httptest`) e il server dell'agente su una porta sua, mai sulla `47115`; come siti, indirizzi inventati (`.example`), mai quelli di Family Manager. Chi cambia cosa fa un file che ha i suoi test cambia anche quelli, nello stesso commit; chi cambia una richiesta la cambia in `server_test.go`, oltre che nella pagina. I test non sostituiscono la prova con il server finto e l'agente compilato, che resta da fare prima di pubblicare.
- **I commenti del codice e i messaggi dei commit sono in inglese**; quello che l'utente legge (domande, messaggi, voci del menu), il README e le note sono in italiano.
- **L'agente si aggiorna da solo, e una release arriva subito sui computer**: chi ha l'agente la scarica al suo avvio successivo e la installa alla chiusura, senza passare da Family Manager e senza che nessuno confermi (o subito, con *Cerca aggiornamenti*: sul Mac nella finestra di *Informazioni*, su Windows nel menu dell'icona). Una versione che non parte non si aggiorna più da sola: va reinstallata a mano. Prima di pubblicare si prova l'agente compilato, e una modifica a `update.go`, `update_darwin.go`, `update_windows.go` o a `version.json` (lo scrive `build.sh`) si prova con un server finto, come dice la nota `Agente dello scanner`, sezione *L'aggiornamento*. Il primo numero della versione si alza solo quando la pagina di prima non riesce più a parlare con l'agente: è l'unico salto che l'agente non fa da solo.
- **L'icona sta in due repository**: `icon.png` qui e `branding/family-manager-agent-icon-512.png` in Family Manager. Cambiando il disegno si sostituiscono tutti e due.

## Documentazione

- "Aggiornare la documentazione" vuol dire il vault **e** il `README.md`, non solo il README.
- **Prima di un commit la documentazione del codice committato dev'essere aggiornata**: si guarda cosa c'è nel commit e si controlla che il README e le note del vault dicano quello che il codice fa adesso. Vale anche quando la richiesta è solo "fai il commit".
- Le regole su come sono scritte le note stanno nel vault e si rileggono prima di scrivere: `_index.md` di Family Manager, sezione *Come sono scritte le note* (niente titolo `#`, collegamenti alle note vicine nelle proprietà, una nota `_index` per cartella). Una nota che esiste si modifica per sostituzioni, e si rilegge subito prima: nel vault lavorano anche altri agenti e il proprietario.
- Il controllo finale delle note è in Family Manager e si lancia da qualunque cartella: `python3 ~/Dev/family-manager-cms/scripts/vault-check.py`. Segnala le note fuori regola senza correggerle. Così com'è guarda le note di Family Manager, che hanno collegamenti a quelle dell'agente; per le note dell'agente gli si passa la loro cartella (`vault-check.py "<vault>/Projects/Family Manager Agent"`) e si guardano solo i gruppi *Titles*, *Indexes* e *Links*: *Extension cards*, *Dependencies between extensions* e *Migrations* riguardano Family Manager e lì compaiono sempre.
- Le cose da fare sull'agente (una funzione nuova, un'idea) sono voci del `ToDo/` della sua cartella del vault, che nasce con la prima voce, e si scrivono con la skill `/feature-analysis` (`.claude/skills/feature-analysis/SKILL.md`; chi non usa le skill di Claude Code legge quel file): prima l'analisi, poi, con le risposte del proprietario, il piano. Non scrive codice. Trova il vault dalle cartelle di `.claude/settings.local.json`, dove per i modelli va aggiunta anche `MyVault/Template`. Una voce che cambia solo la pagina è del ToDo di Family Manager e si scrive da lì, con la sua skill.

## Versioni e rilasci

- Si sviluppa sul branch `dev`, come in Family Manager (dall'11/10/2026): i commit vanno lì. `main` contiene solo le versioni pubblicate e non ci si committa: per pubblicare si porta `dev` in `main` (`git merge --ff-only dev`) e da `main` si lancia `release.sh`, che parte solo da lì e con tutto committato. Poi si torna su `dev`.
- Prima di un commit si controllano `git status` e il branch (`git branch --show-current`), e non si toccano i file non committati che non sono propri.
- Il push rende pubblico il commit: si fa solo quando il proprietario lo chiede, o con `release.sh`.
- La versione è nel file `VERSION`. Va alzata a ogni pubblicazione: la pagina avvisa chi ha un agente più vecchio di quello che Family Manager offre, e gli agenti si aggiornano a quella pubblicata.
- Le versioni sono in `CHANGELOG.md` (SemVer, in italiano; le regole dei numeri sono in cima al file). Ogni modifica a quello che l'agente fa, o a quello che vede chi lo usa, va aggiunta alla sezione `[Non rilasciato]` nello stesso commit; quello che cambia solo per chi lavora qui va sotto *Strumenti*, e quello che va fatto in Family Manager perché la versione funzioni sotto *Per il rilascio*. Per pubblicare, `./close-version.py X.Y.Z` scrive il numero in `VERSION` e chiude la sezione; `release.sh` si ferma se la versione non ha la sua sezione, e ne fa le note della release.
- **Pubblicare qui non basta.** Family Manager offre la versione scritta in `scripts/fetch-agent.mjs` (`VERSION` e le tre impronte, che `release.sh` stampa alla fine): finché lo script non cambia e Family Manager non viene rilasciato, la pagina continua a offrire quella di prima.
