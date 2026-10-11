# Family Manager Agent

Il programma che comanda lo scanner attaccato al computer per conto della pagina *Scansiona* di Family Manager. Il browser da solo non può usare il driver di uno scanner: l'agente sì, con quello che il sistema ha già (*Acquisizione Immagine* sul Mac, WIA su Windows).

È un programma solo, scritto in Go, per **Mac con processore Apple** e **Windows a 64 bit**. Non gira sul server: si scarica dalla pagina e si apre sul computer.

Fino all'11/10/2026 era la cartella `scanner-agent/` del repository di Family Manager. Ora quel repository non ne ha i sorgenti: scarica i due zip di una versione pubblicata qui (`scripts/fetch-agent.mjs`).

## Cosa fa

- Parte quando il sistema apre `familymanageragent://open` (lo fa la pagina); non parte da solo all'accensione.
- Risponde su `http://127.0.0.1:47115`, solo dal computer stesso, e solo ai siti che il proprietario ha accettato: alla prima chiamata di un sito chiede "Permetti a `<sito>` di usare lo scanner?" e ricorda la risposta.
- Fa passare i fogli del caricatore: fronte e retro, colore, 200 punti, A4, un JPEG per facciata, in una cartella temporanea.
- Si chiude dopo dieci minuti senza richieste. All'uscita cancella la cartella temporanea: sul computer non resta nessuna scansione.
- Finché è aperto ha un'icona (barra dei menu sul Mac, accanto all'orologio su Windows) con *Cerca aggiornamenti*, *Dimentica i siti accettati* ed *Esci*.
- Si aggiorna da solo: mentre è aperto cerca una versione più nuova tra le release di questo repository, la scarica, e la mette al suo posto quando si chiude. Dal menu lo si può chiedere subito ([Aggiornarsi da solo](#aggiornarsi-da-solo)).

Non conosce Family Manager: nessun indirizzo, nessuna chiave, nessun token. Il file lo carica la pagina. L'unico indirizzo che chiama è quello delle release di questo repository, su GitHub, per cercare gli aggiornamenti: non manda niente, a parte la richiesta.

## Installare

I due zip sono nelle [release](https://github.com/leicaflorian/family-manager-agent/releases), e la pagina *Scansiona* li fa scaricare. I programmi non sono firmati, quindi la prima volta vanno accettati a mano.

- **Mac**: aprire `FamilyManagerAgent-mac.zip`, spostare *Family Manager Agent* in *Applicazioni* e aprirlo. macOS lo rifiuta: *Impostazioni di Sistema → Privacy e sicurezza → Apri comunque*.
- **Windows**: aprire `FamilyManagerAgent-windows.zip` e poi *Family Manager Agent*. Windows mostra "PC protetto": *Ulteriori informazioni → Esegui comunque*. Il programma si copia da solo in `%LOCALAPPDATA%\FamilyManagerAgent\`: lo zip si può buttare.

## Aggiornarsi da solo

Dalla 1.1.0. Chi ha la 1.0.0 scarica la 1.1.0 a mano, un'ultima volta.

1. Quindici secondi dopo l'avvio, e poi una volta al giorno se resta aperto, l'agente legge `version.json` dell'ultima release (`releases/latest/download/version.json`).
2. Se la versione pubblicata è più alta della sua, scarica lo zip del suo sistema da quella release e ne controlla l'impronta (sha256) con quella scritta in `version.json`. Lo tiene da parte: `~/Library/Caches/FamilyManagerAgent/update/` sul Mac, `%LOCALAPPDATA%\FamilyManagerAgent\update\` su Windows. Nel menu dell'icona compare *Versione X.Y.Z pronta: si installa alla chiusura*.
3. Quando si chiude (da solo dopo dieci minuti, o con *Esci*) mette il programma nuovo al posto del vecchio. Dall'avvio successivo gira la versione nuova. Non chiede niente e non sostituisce niente durante una scansione.

**Dal menu, subito.** *Cerca aggiornamenti* fa lo stesso controllo senza aspettare: se c'è una versione più nuova la scarica, la installa e riavvia l'agente, senza attendere la chiusura. Quando una versione è già stata scaricata la voce diventa *Installa adesso la versione X.Y.Z*. Risponde sempre con una finestra: *Questa è già l'ultima versione*, oppure perché non ha installato (manca la connessione, c'è una scansione in corso, la versione ha un primo numero più alto, l'app non è in *Applicazioni*, la sostituzione non è riuscita).

Cosa non fa:

- **Non passa da solo a un primo numero più alto** (dalla 1.x alla 2.0.0): quel numero cambia quando la pagina di prima non riesce più a parlare con l'agente, e la versione nuova arriva insieme alla pagina, che la fa scaricare.
- **Non installa uno zip con un'impronta diversa** da quella pubblicata, né una release senza impronte (la 1.0.0).
- **Non si aggiorna se non può essere sostituito**: sul Mac l'app deve stare in una cartella in cui si può scrivere, come *Applicazioni*. Aperta da dove è stata scaricata, senza spostarla, macOS la fa girare da una copia di sola lettura.

**Esempio.** Sul Mac c'è la 1.1.0 e viene pubblicata la 1.1.1. Alla prima scansione dopo la pubblicazione l'agente parte, quindici secondi dopo scarica la 1.1.1, e dieci minuti dopo l'ultima richiesta della pagina si chiude e la installa. Alla scansione dopo, la pagina parla con la 1.1.1.

Cosa ha fatto si legge in `agent.log`, nelle righe che cominciano con `update:`. L'impronta protegge da uno zip arrivato rotto o cambiato per strada, non da una release pubblicata da chi non dovrebbe: chi può pubblicare una release in questo repository decide cosa gira sui computer che hanno l'agente.

## Compilare

Da un Mac, con Go (`brew install go`) e gli strumenti da riga di comando di Xcode (`swift` compreso). La prima volta serve la rete: Go scarica le librerie e `go-winres`.

```bash
./build.sh         # i due zip e version.json, in dist/
./build.sh probe   # solo le prove, in build/: scanner-probe e scanner-probe.exe
```

`build.sh` controlla il codice per i due sistemi (`go vet`), compone `Family Manager Agent.app` con la sua icona e la firma in locale (senza certificato), compila `Family Manager Agent.exe` con la sua, e scrive i due zip con `version.json` (la versione, i nomi dei due zip e le loro impronte, che un agente controlla prima di aggiornarsi). Le cartelle `build/` e `dist/` non sono versionate.

**L'icona** è `icon.png`, 512 punti, con lo sfondo di un colore solo. Per cambiarla si sostituisce il file e si ricompila. Dove finisce:

- nell'app del Mac, ridisegnata da `mac-icon.swift` nella forma che macOS vuole (un'icona con un'altra forma macOS 26 la mostra piccola in un riquadro grigio);
- nel file del programma di Windows, con `go-winres`, che scrive `rsrc_windows_amd64.syso` (non versionato);
- accanto all'orologio di Windows, perché il programma porta `icon.png` dentro di sé.

Nella barra dei menu del Mac c'è invece un disegno a un colore, fatto dal codice.

## Pubblicare una versione

Le versioni e cosa contengono sono in [`CHANGELOG.md`](CHANGELOG.md): quello che cambia si scrive lì mentre si lavora, nella sezione *Non rilasciato*, e i numeri seguono le regole scritte in cima a quel file.

1. `./close-version.py X.Y.Z`, con il numero scelto: lo scrive in `VERSION` e, nel changelog, fa diventare *Non rilasciato* la sezione di quella versione, con la data di oggi. Si ferma se *Non rilasciato* è vuoto o se il numero non è più alto dell'ultimo pubblicato.
2. Commit di tutto, su `dev`: è il branch su cui si sviluppa. `main` contiene solo le versioni pubblicate.
3. Portare `dev` in `main`: `git checkout main && git merge --ff-only dev`.
4. `./release.sh`: compila, mette il tag `vX.Y.Z` sul commit, fa il push di `main` e crea la release su GitHub con i due zip e `version.json`, e come note la sezione della versione nel changelog. Si ferma se non si è su `main`, se c'è qualcosa di non committato, se la versione è già pubblicata o se nel changelog non ha la sua sezione.
5. Alla fine stampa le impronte (sha256) dei tre file. In Family Manager vanno scritte, con il numero della versione, in `scripts/fetch-agent.mjs`: finché non lo si fa, Family Manager continua a offrire la versione di prima.
6. Tornare su `dev`: `git checkout dev`.

Serve `gh` (`brew install gh`) con l'accesso fatto, e `python3`, che c'è con gli strumenti di Xcode.

Dal momento in cui la release c'è, gli agenti che si aggiornano da soli la prendono al loro avvio successivo, senza aspettare Family Manager: una versione pubblicata non si ritira, e una correzione è una versione nuova.

## Le richieste

| Richiesta | Cosa risponde |
| --- | --- |
| `GET /status` | `{ version, os, scanner }`, con `scanner` uno tra `ready`, `empty`, `off` |
| `POST /scan` | `202`: fa partire i fogli, in coda alle facciate già arrivate. `409` se una scansione è in corso |
| `GET /pages` | `{ session, state, error, message, pages }`: `state` è `idle`, `scanning`, `done`, `error`; `error` è `off`, `empty`, `jam`, `scanner` |
| `GET /pages/<id>` | Il JPEG di una facciata |
| `DELETE /pages` | `204`: cancella le facciate. `409` durante una scansione |

Senza l'intestazione `Origin`, o con `Host` diverso da `127.0.0.1:47115`: `403 forbidden`. Un sito mai visto: `403 origin_pending` e la domanda sul computer; uno rifiutato: `403 origin_denied`.

La porta `47115` e queste richieste sono quello che lega l'agente alla pagina: chi le cambia qui le cambia anche nella pagina (`extensions/document-reader/src/module/scan/agent.ts` di Family Manager) e nella regola di sicurezza di Directus (`CONTENT_SECURITY_POLICY_DIRECTIVES__CONNECT_SRC`).

## Provare con curl

```bash
open "build/Family Manager Agent.app"                    # oppure: open "familymanageragent://open"
curl -i http://127.0.0.1:47115/status                    # 403: manca il sito che chiama
curl -i -H 'Origin: http://localhost:8056' http://127.0.0.1:47115/status   # la prima volta: la domanda sul Mac
curl -s -H 'Origin: http://localhost:8056' http://127.0.0.1:47115/pages
```

`POST /scan` fa passare i fogli veri: si prova con un foglio qualunque, e alla fine `DELETE /pages`.

Cosa vede e cosa fa l'agente è in `agent.log`, nella cartella delle impostazioni (`~/Library/Application Support/FamilyManagerAgent/` sul Mac, `%AppData%\FamilyManagerAgent\` su Windows), insieme a `origins.json`, i siti accettati. Per una prova senza far comparire la domanda si aggiunge il sito di prova a `origins.json` prima di aprire l'agente; **non** si cambia `HOME`, perché *Acquisizione Immagine* smette di vedere lo scanner.

Su Windows, senza la pagina: `test-windows.cmd`, copiato sul PC accanto al programma. Avvia l'agente, fa passare i fogli del caricatore, scarica le facciate come `pagina-1.jpg`… e scrive `risultato.txt`.

## I file

| File | Cosa contiene |
| --- | --- |
| `main.go` | L'avvio, l'icona, la chiusura da solo, la pulizia all'uscita |
| `server.go` | Le risposte e `guard`, che decide chi passa |
| `origins.go` | I siti accettati |
| `update.go`, `update_darwin.go`, `update_windows.go` | L'aggiornamento: il controllo, lo zip scaricato e la sua impronta, e la sostituzione del programma alla chiusura, sul Mac e su Windows |
| `scanner.go` | La parte comune dello scanner: `Driver`, `Session`, le impostazioni fisse |
| `scanner_darwin.go`, `scanner_darwin.m`, `agent_darwin.h` | Lo scanner sul Mac (`ImageCaptureCore`) |
| `scanner_windows.go` | Lo scanner su Windows (WIA, con `go-ole`): scritto senza poterlo provare dal Mac |
| `dialog_*.go`, `dialog_darwin.m` | La domanda e i messaggi |
| `install_windows.go`, `install_darwin.go` | Su Windows la copia in `%LOCALAPPDATA%\FamilyManagerAgent\` e il collegamento nel registro |
| `icon.go`, `icon.png` | L'icona accanto all'orologio di Windows, e il disegno della barra dei menu del Mac |
| `mac-icon.swift` | L'icona dell'app del Mac, nella forma di macOS |
| `probe.go` | La prova: cosa il sistema vede dello scanner |
| `Info.plist` | Il pacchetto del Mac: identificativo `it.familymanager.agent`, lo schema del collegamento, nessuna icona nel Dock |
| `build.sh`, `release.sh`, `VERSION` | La compilazione, la pubblicazione, il numero della versione |
| `CHANGELOG.md`, `close-version.py` | Le versioni e cosa contengono, e lo script che ne chiude una |
| `test-windows.cmd` | La prova su Windows senza la pagina |
| `CLAUDE.md`, `AGENTS.md`, `.claude/` | Le regole per chi lavora qui, persona o agente (i due file sono identici), e la skill `/release` di Claude Code |

## Cose ancora da provare

- La scansione vera dalla pagina, sul Mac e su Windows.
- Tutta la parte di Windows: lo scanner (WIA), l'icona del programma e quella accanto all'orologio.
- Safari: la pagina è in `https` e l'agente in `http` su `127.0.0.1`.
- La voce *Cerca aggiornamenti* con un clic vero: le finestre e il riavvio dell'agente.
- L'aggiornamento con l'app dentro *Applicazioni*: è stato provato con l'app in un'altra cartella, e macOS può impedire a un programma di modificare le app (*Privacy e sicurezza → Gestione app*). E l'aggiornamento su Windows.
