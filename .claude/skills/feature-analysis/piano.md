# Il piano d'azione

Terza parte di `/feature-analysis`, da leggere solo quando si scrive il piano di una voce (vedi `SKILL.md`, *Quale voce e a che punto è*). Si scrive quando tutti i punti di *Da decidere* hanno una risposta.

Il piano parte dalla nota `Analisi`, non dalla conversazione: quello che l'analisi dice già non si va a rileggere nel vault o nel codice.

1. **Le risposte.** Leggere l'analisi: il proprietario può aver scritto nella nota. Le risposte date nella conversazione si scrivono sotto il loro punto, con la data e con le sue parole ("come da consiglio" se ha accettato il consiglio). Se manca una risposta si chiede solo quella, e non si scrive il piano finché non arriva: una decisione non si inventa.
2. **Lo stato di oggi.** `git status` e `git branch --show-current` (si sviluppa su `dev`): altri agenti possono aver toccato gli stessi file o cambiato branch. Se il piano tocca anche la pagina, lo stesso in `~/Dev/family-manager-cms`. Del codice si apre solo quello che serve per rendere precisi i passi (quale file, quale funzione), nel punto che la nota `Agente dello scanner` indica.
3. **La nota.** Leggere `Template/Template - Piano d'azione.md` e scrivere `Projects/Family Manager Agent/ToDo/<Titolo>/Piano.md`, nella cartella della voce accanto all'analisi, con le stesse sezioni del modello e senza i testi tra `%% … %%`. La riga `←` porta al ToDo dell'agente (`← [[Projects/Family Manager Agent/ToDo/_index|ToDo]]`). Proprietà: `tipo: piano d'azione`, `stato: da iniziare`, `analisi: "[[Projects/Family Manager Agent/ToDo/<Titolo>/Analisi|<Titolo>]]"`, `da controllare` vuota.
4. **Le sezioni.** Il modello è scritto per Family Manager: le righe su migrazioni, estensioni e `npm run build` si tolgono.
   - *Decisioni prese*: le risposte, con la data, e anche quello che l'analisi proponeva e non si fa. L'analisi non si riscrive: resta la proposta com'era.
   - *Passi*: in ordine di esecuzione, uno per riga con la casella, ognuno abbastanza preciso da poterlo spuntare (quale file, quale funzione, per quale sistema). Dove servono: `./build.sh`, che controlla e compila per il Mac e per Windows; i passi nella pagina, detti come passi di Family Manager (altro repository, con le sue regole); la documentazione, cioè la nota `Agente dello scanner`, l'How To e il `README.md`, e il changelog se il repository lo ha (`CLAUDE.md`, sezione *Documentazione*).
   - *Verifica*: prima quello che si prova **senza far passare fogli** (`./build.sh`, `./build.sh probe`, `curl` sulle richieste), poi i passi che fa il proprietario, ognuno con quello che deve vedere: la scansione con i fogli veri, la pagina *Scansiona*, e sul PC Windows tutto quello che il piano tocca lì.
   - *Per il rilascio*: la versione **proposta** dell'agente (la sceglie il proprietario) e la pubblicazione con `/release`; in Family Manager non c'è niente da fare perché la pagina la offra (legge l'ultima versione pubblicata), a meno che la voce cambi anche la pagina o alzi il primo numero (`AGENT_MAJOR`): allora serve il suo rilascio. Se cambia la porta: anche la variabile `CONTENT_SECURITY_POLICY_DIRECTIVES__CONNECT_SRC`, in `docker-compose.yml` e su Railway. Se le due parti devono uscire in un certo ordine, lo si scrive qui.
   - *Fuori dal piano*: quello che resta per una voce successiva.
5. **I collegamenti.** Nell'analisi `piano: "[[Projects/Family Manager Agent/ToDo/<Titolo>/Piano|<Titolo> - piano]]"` e `stato: da iniziare`, lo stesso del piano: lo stato è della voce e nelle due note resta uguale. In `ToDo/_index.md` dell'agente, nella riga della voce (cercata per titolo), si aggiunge il collegamento al piano.

## Alla fine

Un messaggio breve: il percorso della nota, i passi in una riga ciascuno, cosa dovrà controllare il proprietario (con i fogli, nella pagina, sul PC). Lo sviluppo non parte da solo: parte quando lo chiede, segue il piano e ne spunta i passi (`stato: in corso`).
