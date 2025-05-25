// gui.go actualizado con navegación por carpetas funcional y sin reiniciar StartAutoSync
package gui

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"p2pfs/internal/peer"
	"p2pfs/internal/fs"
	"p2pfs/internal/state"
)

func Run(peerSystem *peer.Peer) {
	myApp := app.New()
	myApp.Settings().SetTheme(theme.DarkTheme())
	myWindow := myApp.NewWindow("Sistema Distribuido P2P")
	myWindow.Resize(fyne.NewSize(1200, 700))

	statusLabel := widget.NewLabel("Cargando archivos...")
	selectedLabel := widget.NewLabel("Archivo seleccionado: ninguno")
	syncIcon := widget.NewLabel("✅ Actualizado")

	grid := container.NewGridWithColumns(2)
	machinePanels := make(map[int]*fyne.Container)
	machineStates := make(map[int]*widget.Label)
	machineFileLists := make(map[int]*fyne.Container)
	expandedDirs := make(map[int]map[string]bool)

	scroll := container.NewVScroll(grid)
	scroll.SetMinSize(fyne.NewSize(1000, 600))

	var selectedFile *fs.SelectedFile
	var selectedButton *widget.Button
	localID := peerSystem.Local.ID

	peerChecks := container.NewVBox()
	peerCheckMap := make(map[int]*widget.Check)
	for _, p := range peerSystem.Peers {
		if p.ID == localID {
			continue
		}
		pid := p.ID
		label := fmt.Sprintf("Maq%d (%s:%s)", pid, p.IP, p.Port)
		chk := widget.NewCheck(label, nil)
		peerCheckMap[pid] = chk
		peerChecks.Add(chk)
	}
	selectAllCheck := widget.NewCheck("Todas las máquinas", func(checked bool) {
		for _, chk := range peerCheckMap {
			chk.SetChecked(checked)
		}
	})
	peerChecks.Add(selectAllCheck)

	deleteButton := widget.NewButtonWithIcon("Eliminar", theme.DeleteIcon(), func() {
		if selectedFile == nil {
			statusLabel.SetText("❌ Selecciona un archivo para eliminar.")
			return
		}
		err := fs.DeleteFile(peerSystem, *selectedFile)
		if err != nil {
			statusLabel.SetText("❌ " + err.Error())
		} else {
			statusLabel.SetText("🗑️ Eliminación solicitada.")
		}
	})

	transferButton := widget.NewButtonWithIcon("Transferir", theme.MailForwardIcon(), func() {
		if selectedFile == nil {
			statusLabel.SetText("❌ Selecciona un archivo para transferir.")
			return
		}
		checked := make(map[int]bool)
		for id, chk := range peerCheckMap {
			checked[id] = chk.Checked
		}
		n, err := fs.TransferFile(peerSystem, *selectedFile, checked)
		if err != nil {
			statusLabel.SetText("⚠️ " + err.Error())
		} else {
			statusLabel.SetText(fmt.Sprintf("📤 Archivo enviado a %d máquina(s).", n))
		}
	})

	header := container.NewVBox(
		canvas.NewText("Sistema Distribuido P2P", theme.ForegroundColor()),
		container.NewHBox(deleteButton, transferButton, layout.NewSpacer(), syncIcon),
		container.NewHBox(statusLabel, layout.NewSpacer(), selectedLabel),
		widget.NewSeparator(),
		container.NewVBox(
			widget.NewLabel("Seleccionar destino para transferencia:"),
			peerChecks,
		),
	)

	myWindow.SetContent(container.NewBorder(header, nil, nil, nil, scroll))
	myWindow.Show()

	colors := []color.Color{
		color.NRGBA{R: 180, G: 220, B: 255, A: 255},
		color.NRGBA{R: 200, G: 255, B: 200, A: 255},
		color.NRGBA{R: 255, G: 220, B: 180, A: 255},
		color.NRGBA{R: 255, G: 200, B: 200, A: 255},
	}

	for i, pinfo := range peerSystem.Peers {
		label := fmt.Sprintf("Máquina %d", pinfo.ID)
		if pinfo.ID == localID {
			label += " (Local)"
		}
		title := canvas.NewText(label+fmt.Sprintf(" - %s:%s", pinfo.IP, pinfo.Port), color.White)
		title.TextStyle = fyne.TextStyle{Bold: true}
		title.Alignment = fyne.TextAlignCenter

		stateLbl := widget.NewLabel("Desconocido")
		machineStates[pinfo.ID] = stateLbl
		fileList := container.NewVBox()
		machineFileLists[pinfo.ID] = fileList
		expandedDirs[pinfo.ID] = make(map[string]bool)

		content := container.NewVBox(title, stateLbl, widget.NewSeparator(), fileList)
		border := canvas.NewRectangle(colors[i%len(colors)])
		border.StrokeWidth = 4
		border.StrokeColor = colors[i%len(colors)]
		border.FillColor = color.NRGBA{R: 20, G: 20, B: 20, A: 255}
		border.SetMinSize(fyne.NewSize(500, 250))

		panel := container.NewMax(border, container.NewPadded(content))
		machinePanels[pinfo.ID] = panel
		grid.Add(panel)
	}

	var fileCache = make(map[int][]state.FileInfo)

	var renderFileList func(peerID int)

	fs.StartAutoSync(peerSystem, localID, fs.SyncCallbacks{
		UpdateStatus: func(peerID int, online bool) {
			if online {
				machineStates[peerID].SetText("🟢 En línea")
			} else {
				machineStates[peerID].SetText("🔴 Offline")
			}
		},
		UpdateFileList: func(peerID int, files []state.FileInfo) {
			fileCache[peerID] = files
			renderFileList(peerID)
		},
	})

	renderFileList = func(peerID int) {
		files := fileCache[peerID]
		machineFileLists[peerID].Objects = nil
		allOps := state.GetAllPendingOps()

		for _, file := range files {
			parent := filepath.Dir(file.Name)
			depth := strings.Count(file.Name, "/")
			if depth > 0 && !expandedDirs[peerID][parent] {
				continue
			}

			name := filepath.Base(file.Name)
			mod := file.ModTime.Format("02-Jan 15:04")
			suffix := ""
			for _, ops := range allOps {
				for _, op := range ops {
					if op.TargetID == peerID && op.FilePath == file.Name {
						switch op.Type {
						case "get":
							suffix = " ⏳"
						case "send":
							suffix = " 📤"
						case "delete":
							suffix = " 🗑️"
						}
						break
					}
				}
			}

			indent := strings.Repeat("   ", depth)
			label := fmt.Sprintf("%s%s (%s)%s", indent, name, mod, suffix)
			icon := getIconForFile(name, file.IsDir)

			btn := widget.NewButtonWithIcon(label, icon, nil)
			btn.Alignment = widget.ButtonAlignLeading
			btn.Importance = widget.MediumImportance

			pid := peerID
			fname := file.Name
			thisBtn := btn
			var lastClick time.Time

			var arrow *widget.Button
			if file.IsDir {
				expanded := expandedDirs[pid][fname]
				arrow = widget.NewButton("▸", nil)
				if expanded {
					arrow.SetText("▼")
				}
				arrow.OnTapped = func() {
					expandedDirs[pid][fname] = !expandedDirs[pid][fname]
					renderFileList(pid)
				}
			}

			btn.OnTapped = func() {
				now := time.Now()
				if selectedButton != nil {
					selectedButton.Importance = widget.MediumImportance
					selectedButton.Refresh()
				}
				selectedFile = &fs.SelectedFile{FileName: fname, PeerID: pid}
				selectedButton = thisBtn
				thisBtn.Importance = widget.HighImportance
				thisBtn.Refresh()
				selectedLabel.SetText("Archivo seleccionado: " + name + " (Maq" + strconv.Itoa(pid) + ")")

				if file.IsDir && now.Sub(lastClick) < 500*time.Millisecond {
					expandedDirs[pid][fname] = !expandedDirs[pid][fname]
					renderFileList(pid)
				}
				if pid == localID && now.Sub(lastClick) < 500*time.Millisecond && !file.IsDir {
					go openFile(fname)
				}
				lastClick = now
			}

			row := container.NewHBox()
			if arrow != nil {
				row.Add(arrow)
			}
			row.Add(btn)
			machineFileLists[peerID].Add(row)
		}
		machineFileLists[peerID].Refresh()
	}

	myApp.Run()
}

func openFile(name string) {
	fullPath := filepath.Join("shared", name)
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", fullPath)
	case "darwin":
		cmd = exec.Command("open", fullPath)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", fullPath)
	}
	_ = cmd.Start()
}

func getIconForFile(name string, isDir bool) fyne.Resource {
	if isDir {
		return theme.FolderIcon()
	}
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".txt":
		return theme.DocumentIcon()
	case ".mp3":
		return theme.MediaMusicIcon()
	case ".jpg", ".png":
		return theme.MediaPhotoIcon()
	case ".mp4", ".avi":
		return theme.MediaVideoIcon()
	default:
		return theme.FileIcon()
	}
}