# La prima voce: il ToDo dell'agente

Da leggere una volta sola, quando `Projects/Family Manager Agent/ToDo` non c'è ancora (vedi `analisi.md`, *Indice*). Nella cartella dell'agente una sezione nasce quando ha la sua prima nota: prima di questa voce il ToDo non esiste.

1. Creare `Projects/Family Manager Agent/ToDo/_index.md` con il testo qui sotto, com'è.
2. In `Projects/Family Manager Agent/_index.md`, sezione *Sezioni*, aggiungere la riga:

   ```markdown
   - [[Projects/Family Manager Agent/ToDo/_index|ToDo]]: le cose da fare sull'agente, una cartella per voce.
   ```

3. Poi la cartella della voce e la sua riga nella tabella *Voci*, come dice `analisi.md`.

Le sezioni *Idee ancora da analizzare* e *Fatte (cartella `_Done`)* si aggiungono quando servono, con le colonne che hanno nel ToDo di Family Manager.

## Il testo di `ToDo/_index.md`

```markdown
# ToDo

← [[Projects/Family Manager Agent/_index|Family Manager Agent]]

Le cose da fare su Family Manager Agent. Una voce che cambia solo la pagina *Scansiona* sta nel [[Projects/Family Manager/ToDo/_index|ToDo di Family Manager]].

Come è fatta una voce è scritto lì, e vale anche qui: due note per ogni voce in una cartella con il suo titolo ([[Projects/Family Manager/ToDo/_index#Due note per ogni voce|Due note per ogni voce]]), le proprietà ([[Projects/Family Manager/ToDo/_index#Intestazione delle note|Intestazione delle note]]), la cartella `_Done` e il callout con la data ([[Projects/Family Manager/ToDo/_index#Quando una voce è fatta|Quando una voce è fatta]]). Cambia solo il percorso: il collegamento a una voce di qui si scrive `[[Projects/Family Manager Agent/ToDo/<Titolo>/Analisi|<Titolo>]]`, e le sue note tornano a questo indice.

Dal repository dell'agente le due note le scrive la skill `/feature-analysis`.

## Voci

| Analisi | Cosa risolve | Peso |
| --- | --- | --- |
```
