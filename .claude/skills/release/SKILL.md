---
name: release
description: Pubblica una versione nuova di Family Manager Agent - controlla lo stato, propone il numero, alza VERSION, lancia release.sh e riporta le impronte da scrivere in Family Manager. Da usare quando l'utente chiede di pubblicare o rilasciare l'agente.
argument-hint: "[X.Y.Z]"
disable-model-invocation: true
---

# Pubblicare una versione dell'agente

Il lavoro lo fa `release.sh` (leggi la sua intestazione): compila, mette il tag `vX.Y.Z`, fa il push di `main` e crea la release su GitHub con i due zip e `version.json`. Qui lo si guida dalla conversazione. Quello che esce è pubblico e non si ritira: una versione pubblicata non si pubblica di nuovo.

## 1. Stato di partenza

```sh
git fetch --tags origin
git branch --show-current              # deve essere main
git status --short
cat VERSION
git describe --tags --abbrev=0         # l'ultima versione pubblicata
git log --oneline <ultima versione>..HEAD
gh auth status
```

- **Modifiche non committate**: elencarle all'utente e chiedere se fanno parte della versione. `release.sh` parte solo con tutto committato.
- **Niente di nuovo dall'ultima versione**: dirlo e fermarsi.

## 2. Documentazione

Prima del commit il `README.md` e le note del vault (`Agente dello scanner`, `Compilare e installare l'agente dello scanner`) devono dire quello che il codice fa adesso: vedi `CLAUDE.md`, sezione *Documentazione*.

## 3. Versione

Proporre il numero (SemVer: correzione se si corregge soltanto, minore se c'è qualcosa di nuovo, maggiore se la pagina di prima non riesce più a parlare con l'agente) e chiedere conferma: lo sceglie il proprietario. Se cambia quello che lega l'agente alla pagina (porta, richieste), dirlo qui: va cambiata anche la pagina.

## 4. Compilare e pubblicare

Con il numero confermato:

```sh
./build.sh          # la prova: go vet per i due sistemi e i due programmi
```

Scrivere il numero in `VERSION`, fare il commit su `main`, poi:

```sh
./release.sh
```

Se si ferma dopo aver messo il tag (il push o `gh` non riescono), il tag resta in locale e rilanciandolo risponde che la versione è già pubblicata. Prima di togliere il tag o di finire a mano si guarda cosa è arrivato su GitHub: `git ls-remote --tags origin` e `gh release view vX.Y.Z`.

## 5. Resoconto

- La versione, l'indirizzo della release, le tre impronte (sha256) stampate da `release.sh`.
- **Cosa resta da fare in Family Manager** (`~/Dev/family-manager-cms`), senza il quale la pagina continua a offrire la versione di prima: in `scripts/fetch-agent.mjs` il numero in `VERSION` e le tre impronte, poi `node scripts/fetch-agent.mjs`, il changelog e il commit su `dev`. Quel repository ha le sue regole (`CLAUDE.md`): il passo si fa da lì, oppure da qui se l'utente lo chiede, dopo averle lette.
- Nel vault, nota `Agente dello scanner`: la versione pubblicata e la data, in *Prove in locale* o nei *Punti aperti* che la versione chiude.
