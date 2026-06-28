import { Events } from "@wailsio/runtime";
import * as TranscriptionService from "../bindings/speechtranscriber/transcriptionservice";

// Elementi DOM Status
const statusDot = document.getElementById("status-dot");
const statusText = document.getElementById("status-text");

// Elementi DOM Setup
const setupCard = document.getElementById("setup-card");
const chkSherpa = document.getElementById("chk-sherpa");
const chkModel = document.getElementById("chk-model");
const chkFfmpeg = document.getElementById("chk-ffmpeg");
const btnSetup = document.getElementById("btn-setup");
const setupProgressPanel = document.getElementById("setup-progress-panel");
const setupProgressMsg = document.getElementById("setup-progress-msg");
const setupProgressPct = document.getElementById("setup-progress-pct");
const setupProgressBar = document.getElementById("setup-progress-bar");

// Elementi DOM Trascrizione
const transcribeCard = document.getElementById("transcribe-card");
const inputPath = document.getElementById("input-path");
const selectLang = document.getElementById("select-lang");
const selectFormat = document.getElementById("select-format");
const chkCustomOutput = document.getElementById("chk-custom-output");
const customOutputRow = document.getElementById("custom-output-row");
const customOutputPath = document.getElementById("custom-output-path");
const btnStart = document.getElementById("btn-start");
const transcribeProgressPanel = document.getElementById("transcribe-progress-panel");
const transcribeFileInfo = document.getElementById("transcribe-file-info");
const transcribeProgressPct = document.getElementById("transcribe-progress-pct");
const transcribeProgressBar = document.getElementById("transcribe-progress-bar");
const terminalLog = document.getElementById("terminal-log");
const controlButtons = document.getElementById("control-buttons");

// Opzioni di stato locale
let isFolderMode = false;
let transcriptionStartTime = null;

// 1. Verifica iniziale all'avvio
async function checkStatus() {
    try {
        const status = await TranscriptionService.CheckInstallation();
        const selectedSetupModel = document.getElementById("select-model").value;
        let modelInstalled = false;
        if (selectedSetupModel === "parakeet-v3-multi") {
            modelInstalled = status.parakeetV3Installed;
        } else if (selectedSetupModel === "parakeet-110m") {
            modelInstalled = status.parakeetInstalled;
        } else {
            modelInstalled = status.whisperInstalled;
        }
        
        // Aggiorna icone nel setup
        updateStatusDot(chkSherpa, status.sherpaInstalled);
        updateStatusDot(chkModel, modelInstalled);
        updateStatusDot(chkFfmpeg, status.ffmpegInstalled, true); // warning se manca ffmpeg

        if (status.sherpaInstalled && modelInstalled) {
            // Tutto pronto
            statusDot.className = "status-dot active";
            statusText.innerText = "Pronto";
            
            setupCard.classList.add("hidden");
            transcribeCard.classList.remove("hidden");
        } else {
            // Setup richiesto
            statusDot.className = "status-dot warning";
            statusText.innerText = "Configurazione richiesta";
            
            setupCard.classList.remove("hidden");
            transcribeCard.classList.add("hidden");
        }
    } catch (err) {
        console.error("Errore verifica installazione:", err);
        statusDot.className = "status-dot error";
        statusText.innerText = "Errore connessione backend";
    }
}

function updateStatusDot(element, installed, isOptional = false) {
    if (installed) {
        element.className = "status-dot active";
    } else if (isOptional) {
        element.className = "status-dot warning";
    } else {
        element.className = "status-dot error";
    }
}

// Gestione persistenza modello preferito
const savedModel = localStorage.getItem("preferred-model") || "whisper-tiny";
document.getElementById("select-model").value = savedModel;

// Sincronizza i selettori del modello
function syncModelSelection(value) {
    localStorage.setItem("preferred-model", value);
    document.getElementById("select-model").value = value;
    
    // Disabilita/abilita la lingua a seconda del modello
    if (value === "parakeet-110m") {
        selectLang.value = "en";
        selectLang.disabled = true;
    } else {
        selectLang.disabled = false;
    }
}

// Inizializza stato selettore lingua
syncModelSelection(savedModel);

// Listener per cambio modello in esecuzione
window.onModelChanged = (value) => {
    syncModelSelection(value);
    checkStatus();
};

// Toggle visualizzazione Impostazioni (Setup Card)
window.toggleSettings = () => {
    if (setupCard.classList.contains("hidden")) {
        setupCard.classList.remove("hidden");
        transcribeCard.classList.add("hidden");
    } else {
        // Ritorna alla trascrizione
        checkStatus();
    }
};

// 2. Installazione automatica
window.startSetup = async () => {
    const modelType = document.getElementById("select-model").value;
    btnSetup.disabled = true;
    setupProgressPanel.classList.remove("hidden");
    setupProgressMsg.innerText = "Avvio installazione...";
    
    try {
        await TranscriptionService.DownloadDependencies(modelType);
    } catch (err) {
        setupProgressMsg.innerText = "Errore: " + err;
        btnSetup.disabled = false;
    }
};

// Evento avanzamento download
Events.On("download-progress", (event) => {
    const data = event.data;
    if (data.percentage !== undefined) {
        const pct = Math.round(data.percentage);
        setupProgressPct.innerText = pct + "%";
        setupProgressBar.style.width = pct + "%";
    }
    if (data.message) {
        setupProgressMsg.innerText = data.message;
    }
    if (data.status === "completed") {
        setTimeout(() => {
            checkStatus();
            setupProgressPanel.classList.add("hidden");
            btnSetup.disabled = false;
        }, 3000);
    }
});

// 3. Gestione Form di Trascrizione
window.setSourceType = (isFolder) => {
    isFolderMode = isFolder;
    document.getElementById("opt-file").classList.toggle("active", !isFolder);
    document.getElementById("opt-folder").classList.toggle("active", isFolder);
    
    document.getElementById("lbl-path").innerText = isFolder ? "Cartella multimediale" : "File Audio/Video";
    inputPath.placeholder = isFolder ? "Seleziona una cartella..." : "Seleziona un file...";
    inputPath.value = "";
};

window.browseSource = async () => {
    try {
        const path = await TranscriptionService.SelectPath(isFolderMode);
        if (path) {
            inputPath.value = path;
        }
    } catch (err) {
        console.error(err);
    }
};

window.toggleCustomOutput = (checked) => {
    customOutputRow.classList.toggle("hidden", !checked);
    if (!checked) {
        customOutputPath.value = "";
    }
};

window.browseCustomOutput = async () => {
    try {
        const path = await TranscriptionService.SelectPath(true);
        if (path) {
            customOutputPath.value = path;
        }
    } catch (err) {
        console.error(err);
    }
};

// 4. Esecuzione Trascrizione
window.startTranscription = async () => {
    const path = inputPath.value;
    if (!path) {
        alert(isFolderMode ? "Seleziona prima una cartella sorgente!" : "Seleziona prima un file sorgente!");
        return;
    }

    const options = {
        inputPath: path,
        isFolder: isFolderMode,
        outputFormat: selectFormat.value,
        language: selectLang.value,
        useCustomFolder: chkCustomOutput.checked,
        customFolderPath: customOutputPath.value,
        modelType: document.getElementById("select-model").value
    };

    // Resetta terminale e imposta tempo inizio
    transcriptionStartTime = new Date();
    const startTimeStr = transcriptionStartTime.toLocaleTimeString();
    terminalLog.innerHTML = `<div class="terminal-line system">Avvio del flusso di lavoro alle ore ${startTimeStr}...</div>`;
    
    // Mostra barra progresso
    controlButtons.classList.add("hidden");
    transcribeProgressPanel.classList.remove("hidden");
    
    try {
        await TranscriptionService.Transcribe(options);
    } catch (err) {
        appendLog(`Errore: ${err}`, "error");
        controlButtons.classList.remove("hidden");
        transcribeProgressPanel.classList.add("hidden");
    }
};

window.cancelTranscription = async () => {
    try {
        await TranscriptionService.CancelTranscription();
    } catch (err) {
        console.error(err);
    }
};

// Evento avanzamento trascrizione
Events.On("transcribe-progress", (event) => {
    // Il data è trasmesso come stringa JSON
    const data = JSON.parse(event.data);
    
    if (data.status === "converting") {
        transcribeFileInfo.innerText = `[${data.fileIndex}/${data.totalFiles}] Conversione audio: ${data.currentFile}`;
        transcribeProgressPct.innerText = "Conversione...";
        transcribeProgressBar.style.width = "5%";
        appendLog(`Conversione in formato WAV (16kHz mono): ${data.currentFile}`, "system");
    } 
    else if (data.status === "processing") {
        transcribeFileInfo.innerText = `[${data.fileIndex}/${data.totalFiles}] Trascrizione: ${data.currentFile}`;
        transcribeProgressPct.innerText = Math.round(data.percentage) + "%";
        transcribeProgressBar.style.width = Math.round(data.percentage) + "%";
        if (data.partialText) {
            appendLog(data.partialText);
        }
    } 
    else if (data.status === "completed") {
        appendLog(`Trascrizione completata con successo per: ${data.currentFile}`, "system");
        
        if (data.fileIndex === data.totalFiles) {
            // Finito tutto
            const endTime = new Date();
            const elapsedMs = endTime - transcriptionStartTime;
            const elapsedSeconds = Math.round(elapsedMs / 1000);
            
            const hours = Math.floor(elapsedSeconds / 3600);
            const minutes = Math.floor((elapsedSeconds % 3600) / 60);
            const seconds = elapsedSeconds % 60;
            
            let durationStr = "";
            if (hours > 0) durationStr += `${hours}h `;
            if (minutes > 0 || hours > 0) durationStr += `${minutes}m `;
            durationStr += `${seconds}s`;
            
            const formattedStart = transcriptionStartTime.toLocaleTimeString();
            const formattedEnd = endTime.toLocaleTimeString();
            
            appendLog(`Trascrizione completata in totale!`, "system");
            appendLog(`Ora inizio: ${formattedStart} | Ora fine: ${formattedEnd} | Tempo impiegato: ${durationStr}`, "system");
            
            transcribeFileInfo.innerText = "Trascrizione Completata!";
            transcribeProgressPct.innerText = "100%";
            transcribeProgressBar.style.width = "100%";
            setTimeout(() => {
                controlButtons.classList.remove("hidden");
                transcribeProgressPanel.classList.add("hidden");
            }, 5000); // 5 secondi per mostrare l'esito
        }
    } 
    else if (data.status === "error") {
        appendLog(`Errore: ${data.errorMessage}`, "error");
        
        if (data.fileIndex === data.totalFiles || !data.fileIndex) {
            transcribeFileInfo.innerText = "Errore durante l'operazione";
            setTimeout(() => {
                controlButtons.classList.remove("hidden");
                transcribeProgressPanel.classList.add("hidden");
            }, 5000);
        }
    }
});

function appendLog(text, type = "") {
    const line = document.createElement("div");
    line.className = "terminal-line " + type;
    line.innerText = text;
    terminalLog.appendChild(line);
    
    // Auto-scroll in fondo
    terminalLog.scrollTop = terminalLog.scrollHeight;
}

// Avvia controllo iniziale
checkStatus();
