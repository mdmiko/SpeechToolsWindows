# SpeechTranscriber

SpeechTranscriber è un'applicazione desktop offline per la trascrizione automatica di file audio e video. È sviluppata in **Go** e **Wails v3** per il backend, accoppiata a un frontend interattivo in HTML5/CSS3/JavaScript. 

Il motore di trascrizione offline si basa su **Sherpa-ONNX** (con modelli Whisper e Parakeet di NVIDIA) e utilizza **Silero VAD** per la segmentazione intelligente del parlato, garantendo un'elaborazione veloce ed efficiente anche per file audio di lunga durata (es. registrazioni superiori a un'ora).

---

## Caratteristiche Principali

- 🎙️ **Trascrizione Completamente Offline**: Nessun dato viene inviato a server esterni. La trascrizione avviene interamente sul tuo computer per la massima privacy.
- ⚡ **Ottimizzazione VAD (Voice Activity Detection)**: Sfrutta Silero VAD per suddividere l'audio solo in presenza di voce reale. Questo velocizza drasticamente la decodifica ed evita loop di allucinazione nei modelli Whisper.
- 🗂️ **Elaborazione di Singoli File o Cartelle**: Supporta la trascrizione in batch di intere directory contenenti file multimediali.
- 🛠️ **Pannello Impostazioni Dedicato**:
  - Selezione del modello ASR preferito.
  - Selezione della lingua parlata.
  - Definizione di una cartella di output personalizzata per i file salvati.
- 📊 **Monitoraggio in Tempo Reale**:
  - Trascrizione live divisa per segmenti visibile all'interno di un comodo **Accordion comprimibile**.
  - Calcolo del tempo effettivo impiegato per la trascrizione (con indicazione di orario inizio, fine e durata totale).
- 💾 **Formati di Esportazione**:
  - **SRT** (Sottotitoli standard SubRip)
  - **WebVTT** (Sottotitoli per il Web)
  - **TXT (senza timestamp)**: Testo pulito e continuo.
  - **TXT (con timestamp)**: Testo semplice con indicazione temporale dei segmenti (`[HH:MM:SS.mmm --> HH:MM:SS.mmm]`).
  - **Tutti i formati**: Genera simultaneamente file `.txt`, `.srt` e `.vtt`.

---

## Modelli Supportati

Durante la prima configurazione o tramite le impostazioni, l'app consente di scaricare e installare offline i seguenti modelli ASR:
1. **Whisper Tiny (~75MB - Multilingua)**: Modello OpenAI ottimizzato per CPU. Ottimo equilibrio tra velocità e accuratezza.
2. **NVIDIA Parakeet 110M (~110MB - Solo Inglese)**: Estremamente veloce ed efficiente per la lingua inglese.
3. **NVIDIA Parakeet V3 (~670MB - Multilingua/Italiano)**: Modello avanzato ad alta accuratezza per la lingua italiana e internazionale.

---

## Requisiti di Sistema

- **Sorgente audio/video**: Supporta `.mp3`, `.wav`, `.m4a`, `.mp4`, `.mkv`, `.avi`, `.webm`, `.ogg`.
- **FFmpeg**: Necessario per convertire automaticamente i file multimediali in formato WAV compatibile (16kHz mono). Se non è presente nel PATH di sistema, l'applicazione cercherà un eseguibile `ffmpeg.exe` all'interno della cartella `bin/`.

---

## Struttura del Progetto

```
SpeechToolsWindows/
├── bin/                       # Contiene i binari scaricati (sherpa-onnx, ffmpeg)
├── models/                    # Cartella di download dei modelli ASR e VAD (.onnx)
├── frontend/                  # Codice dell'interfaccia utente
│   ├── public/
│   │   └── style.css          # Fogli di stile personalizzati (dark mode e layout responsive)
│   ├── src/
│   │   └── main.js            # Logica frontend e gestione degli eventi Wails
│   └── index.html             # Layout dell'applicazione
├── main.go                    # Entry point dell'applicazione Go
├── transcription_service.go   # Servizio backend in Go per coordinare FFmpeg e Sherpa-ONNX
└── Taskfile.yml               # Script di automazione per build e sviluppo
```

---

## Istruzioni per lo Sviluppo

### Prerequisiti
- **Go 1.21+**
- **Node.js** (per il frontend)
- Strumento CLI di **Wails v3** (istallato tramite `go install github.com/wailsapp/wails/v3/cmd/wails3@latest`)
- (Opzionale) **Task** per eseguire i comandi da Taskfile.

### Esecuzione in modalità Sviluppo
Avvia l'applicazione con ricaricamento a caldo (hot-reload) sia per il frontend che per il backend:
```bash
wails3 dev
# Oppure utilizzando task
task dev
```

### Compilazione per la Produzione
Per compilare l'eseguibile ottimizzato:
```bash
wails3 build
# Oppure utilizzando task
task build
```
L'eseguibile generato si troverà all'interno della cartella `build/`.
