# L'analisi

Seconda parte di `/feature-analysis`, da leggere solo quando si scrive l'analisi di una voce (vedi `SKILL.md`, *Quale voce e a che punto è*).

## 1. Raccolta, in sola lettura

Prima di scrivere si guarda com'è oggi la cosa, senza cambiare niente (codice, impostazioni dell'agente, scanner). Si parte dalle note che riassumono già il codice, e il codice si apre solo per quello che non dicono.

1. **La nota dell'agente**, `Projects/Family Manager Agent/02 - Documentazione tecnica/Agente dello scanner.md`: il codice file per file, le richieste, chi può chiamare, quando parte e si chiude, lo scanner sul Mac e su Windows, i punti aperti. È lunga: prima i titoli, poi le sezioni che la voce tocca. Se la voce riguarda come si compila, si pubblica o si installa, anche `How To/Compilare e installare l'agente dello scanner.md`.
2. **Cosa vede chi lo usa**: `Projects/Family Manager/01 - Documentazione funzionale/Funzioni/Importazione dei documenti.md`, solo la sezione *Dallo scanner di casa*.
3. **Il codice**, solo per quello che la nota non dice o per controllare una regola su cui la proposta si appoggia: il file che la nota indica (tabella *Files* del `README.md`), nel punto che serve. Niente esplorazioni larghe.
4. **La pagina**, solo se la voce cambia quello che la lega all'agente (porta, richieste, versione): la scheda `Projects/Family Manager/02 - Documentazione tecnica/Estensioni/document-reader.md`, e `extensions/document-reader/src/module/scan/agent.ts` in `~/Dev/family-manager-cms`.
5. **Altre voci** che toccano lo stesso punto: si cercano per parola chiave nei due ToDo (`Projects/Family Manager Agent/ToDo` e `Projects/Family Manager/ToDo`, con le loro `_Done`; `grep -rl`), e si legge solo quello che la ricerca trova. Vanno collegate (con il percorso) o non rifatte.

Non si fanno scansioni di prova (`POST /scan`), non si cambia `HOME`, e non si riporta in una nota il contenuto di `origins.json` o di `agent.log`. Cosa il sistema vede dello scanner lo dice `./build.sh probe`, senza far passare fogli.

## 2. La nota

Leggere `Template/Template - Analisi.md` (insieme alle note della raccolta, se non è già stato fatto) e scrivere `Projects/Family Manager Agent/ToDo/<Titolo>/Analisi.md` con **le stesse sezioni, nello stesso ordine e con gli stessi titoli**. I testi tra `%% … %%` sono le istruzioni per chi scrive: si seguono e si tolgono dalla nota.

Il modello è scritto per Family Manager. Per l'agente cambiano due cose:

- la riga `←` in cima porta al ToDo dell'agente: `← [[Projects/Family Manager Agent/ToDo/_index|ToDo]]`;
- in *Cosa cambia* le righe *Schema*, *Estensioni* e *Dati già presenti* non servono. Al loro posto: **Agente** (quali file, e se la modifica è per il Mac, per Windows o per tutti e due), **Pagina e Family Manager** (cosa va cambiato nell'altro repository, se serve) e **Chi ha già l'agente** (cosa succede sui computer che hanno ancora la versione di prima).

**Proprietà.** Quelle del modello: `tipo: analisi`, `stato: da decidere`, `piano` vuoto. In `funzioni` e `tecnica` le note lette nella raccolta che la voce tocca, una per riga e tra virgolette (`- "[[Agente dello scanner]]"`), ognuna nella proprietà della sua cartella. Senza titolo `#`.

**Da un'idea.** La nota dell'idea (`Analisi.md` con `tipo: idea`) prende la struttura del modello e non se ne crea un'altra. La richiesta originale resta, con chi l'ha fatta e quando, dentro *Il problema*; le proprietà che ha già si tengono e si completano; i punti che l'idea elencava come "da decidere" passano nella sezione *Da decidere*.

**Come si scrive.**

- Italiano semplice, come le altre note. Le cose che l'utente vede si chiamano come nell'interfaccia (*Scansiona*, *Dimentica i siti accettati*), i nomi tecnici vanno tra accenti gravi.
- *Il problema* parte da un caso concreto: cosa è successo, quando, cosa si vede oggi sul computer o nella pagina.
- *Proposta*: una sezione numerata per ogni parte. Ogni regola (tempi, condizioni, cosa risponde l'agente) è scritta per esteso sotto **Regola.** e ha il suo **Esempio.** con dati veri: una richiesta e la sua risposta, una versione, un tempo in secondi.
- **Più automatismi possibile**: se l'agente ha già quello che serve per fare una cosa, la fa da solo e lascia correggere.
- **L'agente non conosce Family Manager** e deve restare così: nessun indirizzo, chiave o token. Una proposta che glieli darebbe va scritta come primo punto di *Da decidere*.
- **Nella pagina, quando si può.** Una regola nella pagina si corregge ricompilando l'estensione; una nell'agente esce solo con una versione nuova del programma, su ogni computer che lo ha. Tra le due, se il risultato è lo stesso, si propone la pagina.
- **Se cambia quello che lega l'agente alla pagina** (porta `47115`, una richiesta, una risposta) lo si dice per esteso: le due parti escono con due rilasci diversi, quindi va scritto in che ordine e cosa vede nel frattempo chi ha l'agente vecchio con la pagina nuova, o il contrario.
- **Windows non si prova dal Mac**: per ogni parte che lo tocca la proposta dice cosa dovrà provare il proprietario sul PC.
- Se serve una versione nuova dell'agente lo si dice, senza numero: il numero lo sceglie il proprietario.
- *Da decidere*: solo le scelte che spettano al proprietario, una per punto. Quello che si può verificare nel codice si verifica, non si chiede. Ogni punto ha le alternative, il **Consiglio** con il motivo, e sotto una riga `**Risposta:**` vuota dove il proprietario scrive.

**Indice.** La voce va nella tabella *Voci* di `Projects/Family Manager Agent/ToDo/_index.md` (colonne *Analisi*, *Cosa risolve*, *Peso*), con il collegamento scritto con il percorso (`[[Projects/Family Manager Agent/ToDo/<Titolo>/Analisi\|<Titolo>]]`). Si cerca la riga per titolo e si cambia solo quella.

Se la cartella `ToDo` dell'agente non c'è ancora (è la prima voce): leggere `indice.md`, qui accanto, e fare quello che dice prima di aggiungere la riga.

## 3. Alla fine

Un messaggio breve, perché il resto è nella nota:

- il titolo della voce e il percorso della nota;
- i punti da decidere, **una riga per punto**: la scelta e il consiglio in poche parole. La proposta e i motivi non si ripetono;
- che può rispondere nella conversazione o scrivere nella nota sotto ogni punto, e che per il piano conviene una conversazione nuova (`/clear`, poi `/feature-analysis <Titolo>`).

Ci si ferma qui: il piano si scrive dopo le risposte.
