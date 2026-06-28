# SpeechTranscriber

SpeechTranscriber è un'applicazione desktop offline per la trascrizione automatica di file audio e video. È sviluppata in **Go** e **Wails v3** per il backend, accoppiata a un frontend interattivo in HTML5/CSS3/JavaScript. 

Il motore di trascrizione offline si basa su **Sherpa-ONNX** (con modelli Whisper e Parakeet di NVIDIA) e utilizza **Silero VAD** per la segmentazione intelligente del parlato, garantendo un'elaborazione veloce ed efficiente anche per file audio di lunga durata (es. registrazioni superiori a un'ora).

---

## Caratteristiche Principali

* 🎙️ **Trascrizione 100% Offline & Privata**
  Nessun dato viene mai inviato all'esterno. Tutto il processo di elaborazione audio e decodifica avviene localmente sul proprio computer, garantendo la massima riservatezza per le proprie registrazioni.

* ⚡ **Algoritmo di Segmentazione Intelligente (VAD)**
  Integra il sistema di Voice Activity Detection **Silero VAD**. Questa tecnologia analizza l'audio e lo suddivide in segmenti focalizzandosi solo dove rileva parlato effettivo. Previene rallentamenti del modello ASR, ottimizza i tempi di calcolo ed evita che Whisper generi "allucinazioni" o cicli di parole ripetute nei momenti di silenzio.

* 🗂️ **Modalità Singola o in Batch (Cartelle)**
  Offre flessibilità sia per elaborare un singolo file audio/video, sia per automatizzare la trascrizione massiva di intere cartelle contenenti più registrazioni.

* ⚙️ **Impostazioni di Configurazione Avanzate**
  Una sezione dedicata permette di regolare a piacimento:
  - Il modello ASR offline preferito (Whisper Tiny, Parakeet 110M, Parakeet V3).
  - La lingua parlata nell'audio (con rilevamento automatico o impostazione fissa).
  - La cartella personalizzata in cui salvare automaticamente i file di testo e i sottotitoli generati.

* 📊 **Monitoraggio e Statistiche Live**
  - I segmenti trascritti vengono visualizzati in tempo reale all'interno di un pratico **Accordion richiudibile** per non ingombrare l'interfaccia.
  - Al termine del processo viene stampato un report dettagliato con **orario esatto di inizio, fine e durata complessiva** dell'elaborazione.

* 💾 **Esportazioni Flessibili in Più Formati**
  - **SRT** (.srt): Sottotitoli standard con millisecondi formattati per i lettori video.
  - **WebVTT** (.vtt): Sottotitoli compatibili con i lettori HTML5 web.
  - **TXT (Solo Testo)**: Un file di testo pulito e continuo ideale da leggere o dare in pasto a strumenti di sintesi.
  - **TXT (Con Timestamp)**: Mantiene indicazioni chiare del tempo per ciascun segmento (`[HH:MM:SS.mmm --> HH:MM:SS.mmm]`).
  - **Tutti i formati**: Genera contemporaneamente tutti i file sopra citati con un solo clic.

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

---

## Credits

Questo progetto è stato reso possibile grazie all'integrazione di eccezionali tecnologie open source:

- **[Wails v3](https://v3.wails.io/)**: Per il fantastico framework desktop leggero basato su Go e motori Web nativi.
- **[Sherpa-ONNX](https://github.com/k2-fsa/sherpa-onnx)**: Il motore di inferenza offline che permette di eseguire modelli ASR senza dipendenze pesanti come Python.
- **[Silero VAD](https://github.com/snakers4/silero-vad)**: Il modello di Voice Activity Detection ad altissime prestazioni per la segmentazione della voce.
- **[OpenAI Whisper](https://github.com/openai/whisper)**: Per l'architettura dei modelli di trascrizione multilingua.
- **[NVIDIA NeMo Parakeet](https://huggingface.co/nvidia)**: Per i modelli ASR basati su CTC/TDT veloci e accurati.
- **[FFmpeg](https://ffmpeg.org/)**: Per la gestione e la conversione dei flussi audio dei file multimediali.

Sviluppato e mantenuto da **[mdmiko](https://github.com/mdmiko)**.
