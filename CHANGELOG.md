# Changelog

Le versioni di Family Manager Agent e cosa contengono. Ogni versione è un tag `vX.Y.Z` con la sua release su GitHub, che ha i due zip da scaricare e `version.json`.

Numeri di versione ([SemVer](https://semver.org/lang/it/)):

- **X** (maggiore): la pagina *Scansiona* di prima non riesce più a parlare con l'agente, perché è cambiata la porta o una richiesta;
- **Y** (minore): l'agente fa qualcosa di nuovo, e la pagina di prima continua a funzionare;
- **Z** (correzione): solo correzioni.

Le novità ancora da pubblicare stanno in **Non rilasciato**: ogni modifica a quello che l'agente fa, o a quello che vede chi lo usa, si aggiunge lì nello stesso commit. Sotto *Strumenti* va quello che cambia solo per chi lavora qui (script, regole). Nella parte *Per il rilascio* vanno le cose da fare in Family Manager perché la versione funzioni, per esempio una richiesta da cambiare anche nella pagina.

Per pubblicare, `./close-version.py X.Y.Z` chiude la versione (il numero lo sceglie il proprietario) e `./release.sh` la pubblica, con le note prese da qui: i passi sono nel `README.md`. Pubblicare non basta: Family Manager offre la versione scritta nel suo `scripts/fetch-agent.mjs`.

## [Non rilasciato]

### Correzioni

- **Su Windows l'agente lascia stare gli scanner di rete**, come sul Mac: guarda solo quelli attaccati al computer. Prima, a ogni domanda sullo stato e a ogni scansione, si collegava a tutti gli scanner che Windows elenca, uno dopo l'altro, finché ne trovava uno con il caricatore: su un PC con una stampante multifunzione sul Wi-Fi, che Windows elenca due volte e prima dello scanner, passava ogni volta da lì, la svegliava, e aspettava la sua risposta. È il motivo per cui Windows risultava lento sullo scanner. Scritto senza poterlo provare.
- **Su Windows la facciata non arriva più ritagliata quando il driver non accetta i 200 punti**: l'agente chiedeva un'area fissa in punti, quella di un A4 a 200, anche quando il driver rifiutava la risoluzione e restava alla sua. Sul PC, l'11/10/2026, arrivavano così i due terzi centrali del foglio. Ora l'area è calcolata sulla risoluzione che il driver ha tenuto: il foglio arriva intero, a quella risoluzione. Scritto senza poterlo provare.

### Novità

- **Su Windows `agent.log` dice a che punto è una scansione**: ogni passo lascia una riga `scan:` (la richiesta, l'attesa del thread di WIA, lo scanner scelto e in quanto tempo si è collegato, le impostazioni che il driver ha tenuto (risoluzione e area comprese), come il caricatore tiene il foglio, ogni facciata con il suo tempo e la sua misura, la fine). Di un valore rifiutato dal driver (`WIA property … refused`) scrive anche il codice dell'errore, il valore che resta e quelli che il driver accetta. Prima, tra `POST /scan` e la prima facciata l'agente non scriveva niente: una scansione che non parte lasciava la pagina su *Scansione in corso…* e il log vuoto, come è successo l'11/10/2026 sul PC. L'ultima riga dice dove si è fermata. Scritto senza poterlo provare.

### Strumenti

- **I primi test automatici**, per i siti accettati (`origins.go`), le richieste e chi può farle (`server.go`), la scansione con uno scanner finto (`scanner.go`) e l'aggiornamento (`update.go`, con la sostituzione dell'app sul Mac): sono i file `_test.go`, accanto al codice. `build.sh` li fa girare prima di compilare, quindi `release.sh` non pubblica se uno fallisce. Non aprono l'agente, non toccano lo scanner e non chiamano la rete. Per poterli scrivere, `Updater` e `Origins` ricevono l'indirizzo delle release, la cartella dello zip, il programma da sostituire e il file delle risposte, invece di calcolarli dove servono: l'agente fa quello che faceva.

## [1.2.1] - 2026-10-11

### Correzioni

- **Su Windows la pagina non dice più che l'agente non risponde quando Windows è lento a rispondere sullo scanner**: `GET /status` chiedeva lo stato a WIA a ogni richiesta e aspettava la risposta, che su un PC è arrivata dopo più dei 12 secondi che la pagina aspetta. Ora risponde subito con l'ultima risposta di WIA, e intanto la fa richiedere (non più spesso di ogni tre secondi). Solo la prima volta aspetta, al massimo sei secondi: se WIA non ha ancora risposto lo scanner risulta spento, e la pagina si corregge da sola quando la risposta arriva. In `agent.log` una risposta di WIA arrivata dopo più di due secondi lascia una riga (`status: WIA answered … after …`, e `WIA: connecting to … took …` per l'apparecchio lento). Scritto senza poterlo provare.

## [1.2.0] - 2026-10-11

### Novità

- **Informazioni, dal menu dell'icona**: apre una finestra che dice cos'è l'agente (comanda lo scanner del computer per la pagina *Scansiona*, risponde solo ai siti accettati, si chiude da solo e non lascia scansioni), quale versione è aperta, e quali siti sono stati accettati o rifiutati.
- **Sul Mac *Cerca aggiornamenti* e *Dimentica i siti accettati* sono pulsanti di quella finestra**, e non più voci del menu, dove restano *Informazioni* ed *Esci*. *Cerca aggiornamenti* lascia la finestra aperta, e la risposta arriva in una finestra sopra; dopo *Dimentica i siti accettati* la finestra si riapre con l'elenco vuoto. Su Windows le due voci restano nel menu, perché lì la finestra ha solo *OK*; la parte di Windows è scritta senza poterla provare.
- **Su Windows, una finestra conferma l'installazione**: aprendo il programma scaricato compare *Installazione riuscita*, con la versione e cosa fare dopo (tornare alla pagina *Scansiona* e premere *Riprova*). Prima compariva solo l'icona accanto all'orologio, che Windows spesso nasconde dietro la freccia. La finestra non compare quando l'agente lo avvia la pagina. Scritta senza poterla provare.
- **Su Windows, il programma scaricato chiede di chiudere l'agente già aperto**: aprendolo a mano mentre un agente è aperto compare *Family Manager Agent è già aperto su questo computer*, con *Sì* e *No*. Con *Sì* l'agente aperto viene chiuso di forza, le facciate che aveva ancora vengono cancellate, e il programma si installa al suo posto. Prima si chiudeva senza dire niente, e restava la versione vecchia. Quando l'agente lo avvia la pagina non chiede niente, come prima. Scritto senza poterlo provare.
- **`agent.log` dice quali chiamate l'agente ha rifiutato**: una chiamata rifiutata con `forbidden` (senza un `Origin` valido, o con un `Host` che non è `127.0.0.1:47115`) lascia una riga `refused:` con il metodo, l'indirizzo chiesto, `Host`, `Origin`, il programma che chiama e i nomi delle intestazioni arrivate. Prima non lasciava traccia, e la pagina la prende per un agente che non risponde. Ogni chiamata diversa è scritta una volta, fino a venti.

### Correzioni

- **Sul Mac l'agente chiude le sue finestre prima di uscire**: con una finestra aperta (*Informazioni*, un messaggio, la domanda "Permetti a…"), *Esci*, la chiusura dopo dieci minuti e il riavvio dopo un aggiornamento chiudono anche quella. Una domanda chiusa così non conta come risposta: il sito verrà chiesto di nuovo.

## [1.1.0] - 2026-10-11

### Novità

- **L'agente si aggiorna da solo**: mentre è aperto cerca una versione più nuova tra le release di questo repository (quindici secondi dopo l'avvio, poi una volta al giorno), scarica lo zip del suo sistema e ne controlla l'impronta. Quando si chiude mette il programma nuovo al posto del vecchio: dall'avvio successivo gira la versione nuova, senza scaricare e accettare niente a mano. Nel menu dell'icona, intanto, compare *Versione X.Y.Z pronta: si installa alla chiusura*. Non passa da solo a una versione con il primo numero più alto, che arriva insieme alla pagina. Provato sul Mac con un'app fuori da *Applicazioni*; su Windows è scritto senza poterlo provare.
- **Cerca aggiornamenti, dal menu dell'icona**: fa subito lo stesso controllo, e se c'è una versione più nuova la installa e riavvia l'agente, senza aspettare che si chiuda. Con una versione già scaricata la voce diventa *Installa adesso la versione X.Y.Z*. Risponde sempre con una finestra: *Questa è già l'ultima versione*, oppure il motivo per cui non ha installato.
- **`version.json` ha le impronte dei due zip** (`sha256`): sono quelle che l'agente controlla prima di installare.

### Per il rilascio

- Chi ha la 1.0.0 scarica questa versione a mano dalla pagina *Scansiona*, un'ultima volta: è da questa che l'agente si aggiorna da solo.
- Da questa versione una release arriva sui computer al primo avvio dell'agente, senza passare da Family Manager: l'agente compilato si prova prima di pubblicarlo.

### Strumenti

- **Changelog**: le versioni sono descritte in questo file. `close-version.py` scrive il numero in `VERSION` e chiude la sezione *Non rilasciato*; `release.sh` si ferma se la versione da pubblicare non ha la sua sezione, e ne usa il testo come note della release su GitHub (prima erano l'elenco dei commit).
- **Branch `dev`**: si sviluppa su `dev`, e `main` contiene solo le versioni pubblicate. Per pubblicare si porta `dev` in `main` e si lancia `release.sh` da lì.
- **Regole per chi lavora qui**: `CLAUDE.md` e `AGENTS.md`, identici, e la skill `/release` di Claude Code, che guida la pubblicazione dalla conversazione.
- **Skill `/feature-analysis`** di Claude Code: scrive nel vault l'analisi di una funzione o di un'idea dell'agente e, dopo le risposte del proprietario, il piano d'azione. Non scrive codice.
- **Documentazione**: le note dell'agente hanno una cartella loro nel vault, `Projects/Family Manager Agent`; prima stavano tra quelle di Family Manager.

## [1.0.0] - 2026-10-11

Prima versione pubblicata da questo repository: fino a questo giorno l'agente era la cartella `scanner-agent/` di Family Manager. La scansione vera dalla pagina, e tutta la parte di Windows, non sono ancora state provate.

### Novità

- **Lo scanner del computer, comandato dalla pagina *Scansiona***: l'agente fa passare i fogli del caricatore, fronte e retro, a colori, a 200 punti, in A4, e dà alla pagina un JPEG per facciata. Usa quello che il sistema ha già: *Acquisizione Immagine* sul Mac, WIA su Windows.
- **Parte quando serve**: lo avvia la pagina, aprendo `familymanageragent://open`; non parte da solo all'accensione. Si chiude dopo dieci minuti senza richieste.
- **Solo i siti accettati**: risponde su `http://127.0.0.1:47115`, solo dal computer stesso. Alla prima chiamata di un sito chiede "Permetti a `<sito>` di usare lo scanner?" e ricorda la risposta; *Dimentica i siti accettati* la cancella.
- **Sul computer non resta nessuna scansione**: le facciate stanno in una cartella temporanea, cancellata all'uscita. L'agente non conosce Family Manager: il file lo carica la pagina.
- **Un'icona finché è aperto**, nella barra dei menu del Mac e accanto all'orologio di Windows, con *Dimentica i siti accettati* ed *Esci*.
- **Per Mac con processore Apple e Windows a 64 bit**, con la sua icona. I programmi non sono firmati: la prima volta vanno accettati a mano.

### Strumenti

- `build.sh` compila i due programmi dal Mac e scrive i due zip con `version.json`; `release.sh` li pubblica come release di GitHub.
- La prova (`./build.sh probe`) dice cosa il sistema vede dello scanner, senza far passare fogli; `test-windows.cmd` prova l'agente su Windows senza la pagina.

[Non rilasciato]: https://github.com/leicaflorian/family-manager-agent/compare/v1.2.1...HEAD
[1.2.1]: https://github.com/leicaflorian/family-manager-agent/compare/v1.2.0...v1.2.1
[1.2.0]: https://github.com/leicaflorian/family-manager-agent/compare/v1.1.0...v1.2.0
[1.1.0]: https://github.com/leicaflorian/family-manager-agent/compare/v1.0.0...v1.1.0
[1.0.0]: https://github.com/leicaflorian/family-manager-agent/releases/tag/v1.0.0
