package main

import (
	"archive/tar"
	"bufio"
	"compress/bzip2"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type InstallationStatus struct {
	SherpaInstalled     bool   `json:"sherpaInstalled"`
	WhisperInstalled    bool   `json:"whisperInstalled"`
	ParakeetInstalled   bool   `json:"parakeetInstalled"`
	ParakeetV3Installed bool   `json:"parakeetV3Installed"`
	FfmpegInstalled     bool   `json:"ffmpegInstalled"`
	AppDir              string `json:"appDir"`
}

type TranscriptionOptions struct {
	InputPath        string `json:"inputPath"`
	IsFolder         bool   `json:"isFolder"`
	OutputFormat     string `json:"outputFormat"` // "txt", "srt", "vtt", "all"
	Language         string `json:"language"`     // "it", "en", "es", "fr", "de", etc.
	UseCustomFolder  bool   `json:"useCustomFolder"`
	CustomFolderPath string `json:"customFolderPath"`
	ModelType        string `json:"modelType"` // "whisper-tiny", "parakeet-110m" o "parakeet-v3-multi"
}

type TranscriptionProgress struct {
	FileIndex    int     `json:"fileIndex"`
	TotalFiles   int     `json:"totalFiles"`
	CurrentFile  string  `json:"currentFile"`
	Percentage   float64 `json:"percentage"`
	Status       string  `json:"status"` // "processing", "converting", "completed", "error"
	PartialText  string  `json:"partialText"`
	ErrorMessage string  `json:"errorMessage"`
}

type TranscriptionService struct {
	activeCmd *exec.Cmd
	cmdMutex  sync.Mutex
	isRunning bool
}

func (s *TranscriptionService) GetAppDir() (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", err
	}
	return filepath.Dir(exePath), nil
}

func (s *TranscriptionService) CheckInstallation() (InstallationStatus, error) {
	appDir, err := s.GetAppDir()
	if err != nil {
		return InstallationStatus{}, err
	}

	sherpaPath := filepath.Join(appDir, "bin", "sherpa-onnx-vad-with-offline-asr.exe")
	sherpaInstalled := fileExists(sherpaPath) && fileExists(filepath.Join(appDir, "models", "silero_vad.onnx"))

	// Controlla se esiste il modello Whisper Tiny
	whisperPath := filepath.Join(appDir, "models", "sherpa-onnx-whisper-tiny", "tiny-encoder.onnx")
	whisperInstalled := fileExists(whisperPath)

	// Controlla se esiste il modello Parakeet 110M
	parakeetPath := filepath.Join(appDir, "models", "sherpa-onnx-nemo-parakeet-110m", "model.int8.onnx")
	parakeetInstalled := fileExists(parakeetPath)

	// Controlla se esiste il modello Parakeet v3 Multilingua
	parakeetV3Path := filepath.Join(appDir, "models", "sherpa-onnx-nemo-parakeet-v3", "encoder.int8.onnx")
	parakeetV3Installed := fileExists(parakeetV3Path)

	// Controlla se ffmpeg è presente nel path di sistema o locale
	_, ffmpegErr := exec.LookPath("ffmpeg")
	ffmpegInstalled := ffmpegErr == nil
	if !ffmpegInstalled {
		ffmpegInstalled = fileExists(filepath.Join(appDir, "bin", "ffmpeg.exe"))
	}

	return InstallationStatus{
		SherpaInstalled:     sherpaInstalled,
		WhisperInstalled:    whisperInstalled,
		ParakeetInstalled:   parakeetInstalled,
		ParakeetV3Installed: parakeetV3Installed,
		FfmpegInstalled:     ffmpegInstalled,
		AppDir:              appDir,
	}, nil
}

// DownloadFile scarica un file mostrando il progresso via eventi Wails
func (s *TranscriptionService) downloadFile(url string, destPath string, eventName string) error {
	out, err := os.Create(destPath + ".tmp")
	if err != nil {
		return err
	}
	defer out.Close()

	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("bad status: %s", resp.Status)
	}

	totalSize, _ := strconv.ParseFloat(resp.Header.Get("Content-Length"), 64)

	// Leggi con progresso
	buffer := make([]byte, 32*1024)
	var downloaded float64 = 0

	app := application.Get()

	for {
		n, readErr := resp.Body.Read(buffer)
		if n > 0 {
			_, writeErr := out.Write(buffer[:n])
			if writeErr != nil {
				return writeErr
			}
			downloaded += float64(n)
			if totalSize > 0 && app != nil {
				percentage := (downloaded / totalSize) * 100
				app.Event.Emit(eventName, map[string]interface{}{
					"percentage": percentage,
					"message":    fmt.Sprintf("Scaricati %.2f MB di %.2f MB", downloaded/(1024*1024), totalSize/(1024*1024)),
				})
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			return readErr
		}
	}

	out.Close()
	return os.Rename(destPath+".tmp", destPath)
}

// DownloadDependencies scarica sherpa-onnx e il modello specificato
func (s *TranscriptionService) DownloadDependencies(modelType string) error {
	appDir, err := s.GetAppDir()
	if err != nil {
		return err
	}

	binDir := filepath.Join(appDir, "bin")
	modelsDir := filepath.Join(appDir, "models")

	_ = os.MkdirAll(binDir, 0755)
	_ = os.MkdirAll(modelsDir, 0755)

	app := application.Get()

	// 1. Scarica e installa i binari di sherpa-onnx se non esistono
	sherpaExePath := filepath.Join(binDir, "sherpa-onnx-vad-with-offline-asr.exe")
	if !fileExists(sherpaExePath) || !fileExists(filepath.Join(binDir, "onnxruntime.dll")) || !fileExists(filepath.Join(binDir, "sherpa-onnx-c-api.dll")) {
		if app != nil {
			app.Event.Emit("download-progress", map[string]interface{}{
				"status":  "downloading_sherpa",
				"message": "Scaricamento dei binari di Sherpa-ONNX (win-x64)...",
			})
		}

		tarPath := filepath.Join(binDir, "sherpa-onnx-win-x64.tar.bz2")
		url := "https://github.com/k2-fsa/sherpa-onnx/releases/download/v1.13.2/sherpa-onnx-v1.13.2-win-x64-shared-MD-Release.tar.bz2"

		err = s.downloadFile(url, tarPath, "download-progress")
		if err != nil {
			return fmt.Errorf("errore download binari: %w", err)
		}

		if app != nil {
			app.Event.Emit("download-progress", map[string]interface{}{
				"status":  "extracting",
				"message": "Estrazione dei binari in corso...",
			})
		}

		err = untar(tarPath, binDir)
		_ = os.Remove(tarPath) // Pulisce il file scaricato
		if err != nil {
			return fmt.Errorf("errore estrazione binari: %w", err)
		}
	}

	// 1.5 Scarica silero_vad.onnx se non esiste
	vadModelPath := filepath.Join(modelsDir, "silero_vad.onnx")
	if !fileExists(vadModelPath) {
		if app != nil {
			app.Event.Emit("download-progress", map[string]interface{}{
				"status":  "downloading_vad",
				"message": "Scaricamento del modello VAD (Silero VAD)...",
			})
		}
		vadURL := "https://github.com/k2-fsa/sherpa-onnx/releases/download/asr-models/silero_vad.onnx"
		err = s.downloadFile(vadURL, vadModelPath, "download-progress")
		if err != nil {
			return fmt.Errorf("errore download Silero VAD: %w", err)
		}
	}

	// 2. Scarica il modello selezionato
	if modelType == "parakeet-v3-multi" {
		modelFolder := filepath.Join(modelsDir, "sherpa-onnx-nemo-parakeet-v3")
		encoderPath := filepath.Join(modelFolder, "encoder.int8.onnx")
		decoderPath := filepath.Join(modelFolder, "decoder.int8.onnx")
		joinerPath := filepath.Join(modelFolder, "joiner.int8.onnx")
		tokensPath := filepath.Join(modelFolder, "tokens.txt")

		if !fileExists(encoderPath) || !fileExists(decoderPath) || !fileExists(joinerPath) || !fileExists(tokensPath) {
			_ = os.MkdirAll(modelFolder, 0755)

			parakeetV3BaseURL := "https://huggingface.co/csukuangfj/sherpa-onnx-nemo-parakeet-tdt-0.6b-v3-int8/resolve/main/"

			modelFiles := []struct {
				url  string
				dest string
			}{
				{parakeetV3BaseURL + "encoder.int8.onnx", encoderPath},
				{parakeetV3BaseURL + "decoder.int8.onnx", decoderPath},
				{parakeetV3BaseURL + "joiner.int8.onnx", joinerPath},
				{parakeetV3BaseURL + "tokens.txt", tokensPath},
			}

			for _, mf := range modelFiles {
				if !fileExists(mf.dest) {
					if app != nil {
						app.Event.Emit("download-progress", map[string]interface{}{
							"status":  "downloading_model",
							"message": fmt.Sprintf("Scaricamento modello Parakeet V3: %s...", filepath.Base(mf.dest)),
						})
					}
					err = s.downloadFile(mf.url, mf.dest, "download-progress")
					if err != nil {
						return fmt.Errorf("errore download modello Parakeet V3 %s: %w", filepath.Base(mf.dest), err)
					}
				}
			}
		}
	} else if modelType == "parakeet-110m" {
		modelFolder := filepath.Join(modelsDir, "sherpa-onnx-nemo-parakeet-110m")
		modelPath := filepath.Join(modelFolder, "model.int8.onnx")
		tokensPath := filepath.Join(modelFolder, "tokens.txt")

		if !fileExists(modelPath) || !fileExists(tokensPath) {
			_ = os.MkdirAll(modelFolder, 0755)

			parakeetBaseURL := "https://huggingface.co/csukuangfj/sherpa-onnx-nemo-parakeet_tdt_ctc_110m-en-36000-int8/resolve/main/"

			modelFiles := []struct {
				url  string
				dest string
			}{
				{parakeetBaseURL + "model.int8.onnx", modelPath},
				{parakeetBaseURL + "tokens.txt", tokensPath},
			}

			for _, mf := range modelFiles {
				if !fileExists(mf.dest) {
					if app != nil {
						app.Event.Emit("download-progress", map[string]interface{}{
							"status":  "downloading_model",
							"message": fmt.Sprintf("Scaricamento modello Parakeet: %s...", filepath.Base(mf.dest)),
						})
					}
					err = s.downloadFile(mf.url, mf.dest, "download-progress")
					if err != nil {
						return fmt.Errorf("errore download modello Parakeet %s: %w", filepath.Base(mf.dest), err)
					}
				}
			}
		}
	} else {
		// Default Whisper Tiny
		modelFolder := filepath.Join(modelsDir, "sherpa-onnx-whisper-tiny")
		encoderPath := filepath.Join(modelFolder, "tiny-encoder.onnx")
		decoderPath := filepath.Join(modelFolder, "tiny-decoder.onnx")
		tokensPath := filepath.Join(modelFolder, "tiny-tokens.txt")

		if !fileExists(encoderPath) || !fileExists(decoderPath) || !fileExists(tokensPath) {
			_ = os.MkdirAll(modelFolder, 0755)

			whisperBaseURL := "https://huggingface.co/csukuangfj/sherpa-onnx-whisper-tiny/resolve/main/"

			modelFiles := []struct {
				url  string
				dest string
			}{
				{whisperBaseURL + "tiny-encoder.onnx", encoderPath},
				{whisperBaseURL + "tiny-decoder.onnx", decoderPath},
				{whisperBaseURL + "tiny-tokens.txt", tokensPath},
			}

			for _, mf := range modelFiles {
				if !fileExists(mf.dest) {
					if app != nil {
						app.Event.Emit("download-progress", map[string]interface{}{
							"status":  "downloading_model",
							"message": fmt.Sprintf("Scaricamento modello Whisper: %s...", filepath.Base(mf.dest)),
						})
					}
					err = s.downloadFile(mf.url, mf.dest, "download-progress")
					if err != nil {
						return fmt.Errorf("errore download modello Whisper %s: %w", filepath.Base(mf.dest), err)
					}
				}
			}
		}
	}

	if app != nil {
		app.Event.Emit("download-progress", map[string]interface{}{
			"status":  "completed",
			"message": "Installazione completata con successo!",
		})
	}

	return nil
}

// SelectPath apre un dialogo nativo di Windows per selezionare file o cartelle
func (s *TranscriptionService) SelectPath(selectFolder bool) (string, error) {
	app := application.Get()
	if app == nil {
		return "", fmt.Errorf("app not initialized")
	}

	if selectFolder {
		return app.Dialog.OpenFile().
			SetTitle("Seleziona Cartella").
			CanChooseDirectories(true).
			CanChooseFiles(false).
			PromptForSingleSelection()
	}

	return app.Dialog.OpenFile().
		SetTitle("Seleziona File Multimediale").
		AddFilter("File multimediali", "*.wav;*.mp3;*.m4a;*.mp4;*.mkv;*.avi;*.webm;*.ogg").
		PromptForSingleSelection()
}

// Transcribe avvia la trascrizione per il file o la cartella specificata
func (s *TranscriptionService) Transcribe(options TranscriptionOptions) error {
	s.cmdMutex.Lock()
	if s.isRunning {
		s.cmdMutex.Unlock()
		return fmt.Errorf("un'operazione di trascrizione è già in esecuzione")
	}
	s.isRunning = true
	s.cmdMutex.Unlock()

	go s.runTranscriptionWorkflow(options)
	return nil
}

func (s *TranscriptionService) CancelTranscription() error {
	s.cmdMutex.Lock()
	defer s.cmdMutex.Unlock()

	if s.activeCmd != nil && s.activeCmd.Process != nil {
		// Su Windows dobbiamo terminare il processo in modo pulito
		err := s.activeCmd.Process.Kill()
		s.isRunning = false
		return err
	}
	return nil
}

// Workflow principale eseguito in goroutine
func (s *TranscriptionService) runTranscriptionWorkflow(options TranscriptionOptions) {
	app := application.Get()
	emitProgress := func(p TranscriptionProgress) {
		if app != nil {
			bytes, _ := json.Marshal(p)
			app.Event.Emit("transcribe-progress", string(bytes))
		}
	}

	defer func() {
		s.cmdMutex.Lock()
		s.isRunning = false
		s.activeCmd = nil
		s.cmdMutex.Unlock()
	}()

	// 1. Trova i file da elaborare
	var files []string
	if options.IsFolder {
		err := filepath.Walk(options.InputPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && isSupportedFile(path) {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			emitProgress(TranscriptionProgress{Status: "error", ErrorMessage: "Errore durante la scansione della cartella: " + err.Error()})
			return
		}
	} else {
		if isSupportedFile(options.InputPath) {
			files = append(files, options.InputPath)
		}
	}

	totalFiles := len(files)
	if totalFiles == 0 {
		emitProgress(TranscriptionProgress{Status: "error", ErrorMessage: "Nessun file audio/video supportato trovato."})
		return
	}

	appDir, _ := s.GetAppDir()

	// Esegui per ciascun file
	for i, file := range files {
		currentFileName := filepath.Base(file)
		baseName := strings.TrimSuffix(currentFileName, filepath.Ext(currentFileName))
		emitProgress(TranscriptionProgress{
			FileIndex:   i + 1,
			TotalFiles:  totalFiles,
			CurrentFile: currentFileName,
			Percentage:  0,
			Status:      "converting",
		})

		// Determina cartella di output
		outputDir := filepath.Dir(file)
		if options.UseCustomFolder && options.CustomFolderPath != "" {
			outputDir = options.CustomFolderPath
			_ = os.MkdirAll(outputDir, 0755)
		}

		// Converte in WAV (16kHz, mono) temporaneo
		tempWavPath := filepath.Join(outputDir, fmt.Sprintf("debug_temp_%s.wav", baseName))
		defer os.Remove(tempWavPath) // Pulizia automatica al termine

		err := convertToWav(file, tempWavPath, appDir, func(pct float64) {
			emitProgress(TranscriptionProgress{
				FileIndex:   i + 1,
				TotalFiles:  totalFiles,
				CurrentFile: currentFileName,
				Percentage:  pct,
				Status:      "converting",
			})
		})
		if err != nil {
			emitProgress(TranscriptionProgress{
				FileIndex:    i + 1,
				TotalFiles:   totalFiles,
				CurrentFile:  currentFileName,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Errore conversione audio per %s: %s", currentFileName, err.Error()),
			})
			continue
		}

		emitProgress(TranscriptionProgress{
			FileIndex:   i + 1,
			TotalFiles:  totalFiles,
			CurrentFile: currentFileName,
			Percentage:  10,
			Status:      "processing",
		})

		// Configura argomenti di sherpa-onnx (VAD nativo: segmenta il parlato e
		// gestisce file di lunga durata senza caricare tutto in memoria).
		sherpaExe := filepath.Join(appDir, "bin", "sherpa-onnx-vad-with-offline-asr.exe")
		if !fileExists(sherpaExe) {
			emitProgress(TranscriptionProgress{
				FileIndex:    i + 1,
				TotalFiles:   totalFiles,
				CurrentFile:  currentFileName,
				Status:       "error",
				ErrorMessage: "sherpa-onnx-vad-with-offline-asr.exe non trovato nella cartella bin.",
			})
			continue
		}

		sileroVadPath := filepath.Join(appDir, "models", "silero_vad.onnx")
		var args []string
		if options.ModelType == "parakeet-v3-multi" {
			modelFolder := filepath.Join(appDir, "models", "sherpa-onnx-nemo-parakeet-v3")
			args = []string{
				fmt.Sprintf("--silero-vad-model=%s", sileroVadPath),
				fmt.Sprintf("--encoder=%s", filepath.Join(modelFolder, "encoder.int8.onnx")),
				fmt.Sprintf("--decoder=%s", filepath.Join(modelFolder, "decoder.int8.onnx")),
				fmt.Sprintf("--joiner=%s", filepath.Join(modelFolder, "joiner.int8.onnx")),
				fmt.Sprintf("--tokens=%s", filepath.Join(modelFolder, "tokens.txt")),
				"--num-threads=4",
				tempWavPath,
			}
		} else if options.ModelType == "parakeet-110m" {
			modelFolder := filepath.Join(appDir, "models", "sherpa-onnx-nemo-parakeet-110m")
			args = []string{
				fmt.Sprintf("--silero-vad-model=%s", sileroVadPath),
				fmt.Sprintf("--nemo-ctc-model=%s", filepath.Join(modelFolder, "model.int8.onnx")),
				fmt.Sprintf("--tokens=%s", filepath.Join(modelFolder, "tokens.txt")),
				"--num-threads=4",
				tempWavPath,
			}
		} else {
			modelFolder := filepath.Join(appDir, "models", "sherpa-onnx-whisper-tiny")
			args = []string{
				fmt.Sprintf("--silero-vad-model=%s", sileroVadPath),
				fmt.Sprintf("--whisper-encoder=%s", filepath.Join(modelFolder, "tiny-encoder.onnx")),
				fmt.Sprintf("--whisper-decoder=%s", filepath.Join(modelFolder, "tiny-decoder.onnx")),
				fmt.Sprintf("--tokens=%s", filepath.Join(modelFolder, "tiny-tokens.txt")),
				fmt.Sprintf("--whisper-language=%s", options.Language),
				"--whisper-task=transcribe",
				"--num-threads=4",
				tempWavPath,
			}
		}

		ctx, cancel := context.WithCancel(context.Background())
		cmd := exec.CommandContext(ctx, sherpaExe, args...)
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} // Nasconde la finestra console su Windows

		s.cmdMutex.Lock()
		s.activeCmd = cmd
		s.cmdMutex.Unlock()

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			cancel()
			emitProgress(TranscriptionProgress{Status: "error", ErrorMessage: "Errore pipe stdout: " + err.Error()})
			return
		}

		stderr, err := cmd.StderrPipe()
		if err != nil {
			cancel()
			emitProgress(TranscriptionProgress{Status: "error", ErrorMessage: "Errore pipe stderr: " + err.Error()})
			return
		}

		if err := cmd.Start(); err != nil {
			cancel()
			emitProgress(TranscriptionProgress{Status: "error", ErrorMessage: "Errore avvio sherpa-onnx: " + err.Error()})
			return
		}

		// Legge output (segmenti VAD temporizzati) e streama i parziali al frontend
		var fullTranscriptionBuilder strings.Builder
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				text := scanner.Text()
				if strings.TrimSpace(text) != "" {
					fullTranscriptionBuilder.WriteString(text + "\n")
					partialText := text
					if options.OutputFormat == "txt" {
						partialText = removeTimestamps(text)
					}
					emitProgress(TranscriptionProgress{
						FileIndex:   i + 1,
						TotalFiles:  totalFiles,
						CurrentFile: currentFileName,
						Percentage:  50,
						Status:      "processing",
						PartialText: partialText,
					})
				}
			}
		}()

		var stderrBuilder strings.Builder
		go func() {
			_, _ = io.Copy(&stderrBuilder, stderr)
		}()

		cmdErr := cmd.Wait()
		cancel()

		s.cmdMutex.Lock()
		wasCancelled := !s.isRunning // Se s.isRunning è false, l'utente ha annullato
		s.cmdMutex.Unlock()

		if wasCancelled {
			emitProgress(TranscriptionProgress{Status: "error", ErrorMessage: "Operazione annullata dall'utente."})
			return
		}

		if cmdErr != nil {
			emitProgress(TranscriptionProgress{
				FileIndex:    i + 1,
				TotalFiles:   totalFiles,
				CurrentFile:  currentFileName,
				Status:       "error",
				ErrorMessage: fmt.Sprintf("Errore decodifica: %s. Dettagli: %s", cmdErr.Error(), stderrBuilder.String()),
			})
			continue
		}

		rawText := fullTranscriptionBuilder.String()
		err = saveTranscriptionFiles(rawText, outputDir, baseName, options.OutputFormat)
		if err != nil {
			emitProgress(TranscriptionProgress{
				FileIndex:    i + 1,
				TotalFiles:   totalFiles,
				CurrentFile:  currentFileName,
				Status:       "error",
				ErrorMessage: "Errore salvataggio file: " + err.Error(),
			})
			continue
		}

		emitProgress(TranscriptionProgress{
			FileIndex:   i + 1,
			TotalFiles:  totalFiles,
			CurrentFile: currentFileName,
			Percentage:  100,
			Status:      "completed",
		})
	}
}

// Verifica se l'estensione del file è supportata
func isSupportedFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	supported := map[string]bool{
		".wav":  true,
		".mp3":  true,
		".m4a":  true,
		".mp4":  true,
		".mkv":  true,
		".avi":  true,
		".webm": true,
		".ogg":  true,
	}
	return supported[ext]
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false
	}
	return !info.IsDir()
}

// Esegue la conversione tramite ffmpeg.exe (se presente) o avvisa, calcolando il progresso in tempo reale
func convertToWav(inputPath, outputPath, appDir string, onProgress func(pct float64)) error {
	// Cerca ffmpeg in PATH o locale nella cartella bin
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		ffmpegPath = filepath.Join(appDir, "bin", "ffmpeg.exe")
		if !fileExists(ffmpegPath) {
			return fmt.Errorf("FFmpeg non trovato. Installa FFmpeg o posiziona ffmpeg.exe nella cartella 'bin'")
		}
	}

	// Comando di conversione: 16kHz, mono, formato PCM a 16 bit
	cmd := exec.Command(ffmpegPath, "-y", "-i", inputPath, "-vn", "-acodec", "pcm_s16le", "-ar", "16000", "-ac", "1", outputPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow: true,
	}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return err
	}

	durationRegex := regexp.MustCompile(`Duration:\s*(\d{2}):(\d{2}):(\d{2}(?:\.\d+)?)`)
	timeRegex := regexp.MustCompile(`time=\s*(\d{2}):(\d{2}):(\d{2}(?:\.\d+)?)`)

	var totalSeconds float64 = 0

	scanner := bufio.NewScanner(stderr)
	for scanner.Scan() {
		line := scanner.Text()

		// 1. Trova durata totale
		if totalSeconds == 0 {
			if matches := durationRegex.FindStringSubmatch(line); len(matches) >= 4 {
				h, _ := strconv.ParseFloat(matches[1], 64)
				m, _ := strconv.ParseFloat(matches[2], 64)
				s, _ := strconv.ParseFloat(matches[3], 64)
				totalSeconds = h*3600 + m*60 + s
			}
		}

		// 2. Trova avanzamento temporale
		if totalSeconds > 0 {
			if matches := timeRegex.FindStringSubmatch(line); len(matches) >= 4 {
				h, _ := strconv.ParseFloat(matches[1], 64)
				m, _ := strconv.ParseFloat(matches[2], 64)
				s, _ := strconv.ParseFloat(matches[3], 64)
				currentSeconds := h*3600 + m*60 + s

				pct := (currentSeconds / totalSeconds) * 100
				if pct > 100 {
					pct = 100
				}
				if onProgress != nil {
					onProgress(pct)
				}
			}
		}
	}

	return cmd.Wait()
}

// Salva la trascrizione in vari formati (.txt, .srt, .vtt)
func saveTranscriptionFiles(rawText, outputDir, baseName, format string) error {
	// Pulisce il testo e rimuove i tag temporali interni se presenti per il .txt
	cleanText := removeTimestamps(rawText)

	writeTxt := format == "txt" || format == "all"
	writeTxtTime := format == "txt-time"
	writeSrt := format == "srt" || format == "all"
	writeVtt := format == "vtt" || format == "all"

	if writeTxt {
		txtPath := filepath.Join(outputDir, baseName+".txt")
		err := os.WriteFile(txtPath, []byte(cleanText), 0644)
		if err != nil {
			return err
		}
	}

	if writeTxtTime {
		txtPath := filepath.Join(outputDir, baseName+".txt")
		err := os.WriteFile(txtPath, []byte(rawText), 0644)
		if err != nil {
			return err
		}
	}

	// Se il modello restituisce già timestamp (formato SRT o VTT nativo da sherpa) salviamo quelli.
	// Altrimenti, per Whisper, il testo restituito da sherpa-onnx-offline contiene i timestamp nel formato:
	// [00:12.300 --> 00:15.200] Testo della frase
	// Possiamo fare il parsing di queste righe per generare SRT o VTT corretti.
	if writeSrt {
		srtContent := convertToSRT(rawText)
		srtPath := filepath.Join(outputDir, baseName+".srt")
		err := os.WriteFile(srtPath, []byte(srtContent), 0644)
		if err != nil {
			return err
		}
	}

	if writeVtt {
		vttContent := convertToVTT(rawText)
		vttPath := filepath.Join(outputDir, baseName+".vtt")
		err := os.WriteFile(vttPath, []byte(vttContent), 0644)
		if err != nil {
			return err
		}
	}

	return nil
}

// Helper per convertire secondi float in formato SRT (HH:MM:SS,mmm)
func formatSecondsToSRTTime(sec float64) string {
	hours := int(sec) / 3600
	minutes := (int(sec) % 3600) / 60
	seconds := int(sec) % 60
	ms := int((sec - float64(int(sec))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d,%03d", hours, minutes, seconds, ms)
}

// Helper per convertire secondi float in formato WebVTT (HH:MM:SS.mmm)
func formatSecondsToVTTTime(sec float64) string {
	hours := int(sec) / 3600
	minutes := (int(sec) % 3600) / 60
	seconds := int(sec) % 60
	ms := int((sec - float64(int(sec))) * 1000)
	return fmt.Sprintf("%02d:%02d:%02d.%03d", hours, minutes, seconds, ms)
}

// Rimuove i timestamp tipo [00:12.100 --> 00:15.300] o 154.182 -- 262.684: o righe SRT/VTT per produrre testo pulito
func removeTimestamps(input string) string {
	lines := strings.Split(input, "\n")
	var cleanLines []string

	// Regex per rilevare vari tipi di timestamp o metadati temporali
	reTimeSRTVTT := regexp.MustCompile(`\d{2}(?::\d{2})?:\d{2}[.,]\d{3}\s*-->\s*\d{2}(?::\d{2})?:\d{2}[.,]\d{3}`)
	reTimeBrackets := regexp.MustCompile(`\[\d{2}(?::\d{2})?:\d{2}\.\d{3}\s*-->\s*\d{2}(?::\d{2})?:\d{2}\.\d{3}\]`)
	reTimeParakeet := regexp.MustCompile(`\d+(?:\.\d+)?\s*--\s*\d+(?:\.\d+)?:\s*`)
	reJustNumber := regexp.MustCompile(`^\s*\d+\s*$`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Salta numeri di segmento (SRT)
		if reJustNumber.MatchString(trimmed) {
			continue
		}
		// Salta righe con intervalli di tempo SRT/VTT
		if reTimeSRTVTT.MatchString(trimmed) {
			continue
		}
		// Rimuove tag tra parentesi
		if reTimeBrackets.MatchString(line) {
			line = reTimeBrackets.ReplaceAllString(line, "")
		}
		// Rimuove tag di Parakeet
		if reTimeParakeet.MatchString(line) {
			line = reTimeParakeet.ReplaceAllString(line, "")
		}

		finalLine := strings.TrimSpace(line)
		if finalLine != "" {
			cleanLines = append(cleanLines, finalLine)
		}
	}

	return strings.Join(cleanLines, "\n")
}

// Converte l'output di sherpa con i timestamp in formato SRT standard
func convertToSRT(input string) string {
	lines := strings.Split(input, "\n")
	var srtLines []string
	counter := 1

	// Regex per formato Whisper: [MM:SS.mmm --> MM:SS.mmm] o [HH:MM:SS.mmm --> HH:MM:SS.mmm]
	reWhisper := regexp.MustCompile(`\[(?:(\d{2}):)?(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(?:(\d{2}):)?(\d{2}):(\d{2})\.(\d{3})\]\s*(.*)`)
	// Regex per formato Parakeet: 154.182 -- 262.684: Testo
	reParakeet := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*--\s*(\d+(?:\.\d+)?):\s*(.*)`)

	for _, line := range lines {
		if matches := reWhisper.FindStringSubmatch(line); len(matches) >= 10 {
			hrStart := matches[1]
			if hrStart == "" {
				hrStart = "00"
			}
			minStart, secStart, msStart := matches[2], matches[3], matches[4]

			hrEnd := matches[5]
			if hrEnd == "" {
				hrEnd = "00"
			}
			minEnd, secEnd, msEnd := matches[6], matches[7], matches[8]
			text := matches[9]

			srtStart := fmt.Sprintf("%s:%s:%s,%s", hrStart, minStart, secStart, msStart)
			srtEnd := fmt.Sprintf("%s:%s:%s,%s", hrEnd, minEnd, secEnd, msEnd)

			srtLines = append(srtLines, strconv.Itoa(counter))
			srtLines = append(srtLines, fmt.Sprintf("%s --> %s", srtStart, srtEnd))
			srtLines = append(srtLines, text)
			srtLines = append(srtLines, "") // Riga vuota separatrice
			counter++
		} else if matches := reParakeet.FindStringSubmatch(line); len(matches) >= 4 {
			startSec, _ := strconv.ParseFloat(matches[1], 64)
			endSec, _ := strconv.ParseFloat(matches[2], 64)
			text := matches[3]

			srtStart := formatSecondsToSRTTime(startSec)
			srtEnd := formatSecondsToSRTTime(endSec)

			srtLines = append(srtLines, strconv.Itoa(counter))
			srtLines = append(srtLines, fmt.Sprintf("%s --> %s", srtStart, srtEnd))
			srtLines = append(srtLines, text)
			srtLines = append(srtLines, "")
			counter++
		} else if strings.TrimSpace(line) != "" && !strings.Contains(line, "-->") && !strings.Contains(line, "--") {
			// Riga di testo senza timestamp, la accodiamo all'ultimo blocco se esiste
			if len(srtLines) > 0 {
				lastIdx := len(srtLines) - 2
				if lastIdx >= 0 {
					srtLines[lastIdx] = srtLines[lastIdx] + " " + strings.TrimSpace(line)
				}
			}
		}
	}

	if len(srtLines) == 0 {
		return fmt.Sprintf("1\n00:00:00,000 --> 00:05:00,000\n%s\n", input)
	}

	return strings.Join(srtLines, "\n")
}

// Converte l'output in formato WebVTT standard
func convertToVTT(input string) string {
	lines := strings.Split(input, "\n")
	var vttLines []string
	vttLines = append(vttLines, "WEBVTT")
	vttLines = append(vttLines, "")

	reWhisper := regexp.MustCompile(`\[(?:(\d{2}):)?(\d{2}):(\d{2})\.(\d{3})\s*-->\s*(?:(\d{2}):)?(\d{2}):(\d{2})\.(\d{3})\]\s*(.*)`)
	reParakeet := regexp.MustCompile(`(\d+(?:\.\d+)?)\s*--\s*(\d+(?:\.\d+)?):\s*(.*)`)

	for _, line := range lines {
		if matches := reWhisper.FindStringSubmatch(line); len(matches) >= 10 {
			hrStart := matches[1]
			if hrStart == "" {
				hrStart = "00"
			}
			minStart, secStart, msStart := matches[2], matches[3], matches[4]

			hrEnd := matches[5]
			if hrEnd == "" {
				hrEnd = "00"
			}
			minEnd, secEnd, msEnd := matches[6], matches[7], matches[8]
			text := matches[9]

			vttStart := fmt.Sprintf("%s:%s:%s.%s", hrStart, minStart, secStart, msStart)
			vttEnd := fmt.Sprintf("%s:%s:%s.%s", hrEnd, minEnd, secEnd, msEnd)

			vttLines = append(vttLines, fmt.Sprintf("%s --> %s", vttStart, vttEnd))
			vttLines = append(vttLines, text)
			vttLines = append(vttLines, "")
		} else if matches := reParakeet.FindStringSubmatch(line); len(matches) >= 4 {
			startSec, _ := strconv.ParseFloat(matches[1], 64)
			endSec, _ := strconv.ParseFloat(matches[2], 64)
			text := matches[3]

			vttStart := formatSecondsToVTTTime(startSec)
			vttEnd := formatSecondsToVTTTime(endSec)

			vttLines = append(vttLines, fmt.Sprintf("%s --> %s", vttStart, vttEnd))
			vttLines = append(vttLines, text)
			vttLines = append(vttLines, "")
		}
	}

	if len(vttLines) <= 2 {
		return fmt.Sprintf("WEBVTT\n\n00:00.000 --> 05:00.000\n%s\n", input)
	}

	return strings.Join(vttLines, "\n")
}

// Override temporaneo per evitare librerie esterne complesse per OS runtime info
func getTargetOSArch() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

func untar(tarPath, destDir string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	bz2Reader := bzip2.NewReader(f)
	tarReader := tar.NewReader(bz2Reader)

	for {
		header, err := tarReader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		filename := filepath.Base(header.Name)
		if filename == "sherpa-onnx-offline.exe" || filename == "sherpa-onnx-vad-with-offline-asr.exe" || filename == "onnxruntime.dll" || filename == "sherpa-onnx-c-api.dll" {
			targetPath := filepath.Join(destDir, filename)
			outFile, err := os.Create(targetPath)
			if err != nil {
				return err
			}
			if _, err := io.Copy(outFile, tarReader); err != nil {
				outFile.Close()
				return err
			}
			outFile.Close()

			// Rendi eseguibile l'exe su sistemi non Windows (anche se siamo su Windows, è buona norma)
			if strings.HasSuffix(filename, ".exe") {
				_ = os.Chmod(targetPath, 0755)
			}
		}
	}
	return nil
}
