package gui

import (
    "encoding/base64"
    "encoding/json"
    "fmt"
    "net"
    "os"
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
    "fyne.io/fyne/v2/dialog"


    "p2pfs/internal/peer"
    "p2pfs/internal/fs"
	logger "p2pfs/internal/log"

)

// Estructura para almacenar el archivo seleccionado
type SelectedFile struct {
    FileName string
    PeerID   int
}

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

	scroll := container.NewVScroll(grid)
	scroll.SetMinSize(fyne.NewSize(1000, 600))

	var selectedFile *SelectedFile
	var selectedButton *widget.Button
	localID := peerSystem.Local.ID

	// 🔘 Checkboxes para selección de máquinas
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
		if selectedFile.PeerID == localID {
			err := os.Remove(filepath.Join("shared", selectedFile.FileName))
			if err != nil {
				statusLabel.SetText("❌ Error al eliminar archivo local.")
				return
			}
			statusLabel.SetText("🗑️ Archivo eliminado localmente.")
		} else {
			for _, peer := range peerSystem.Peers {
				if peer.ID == selectedFile.PeerID {
					go deleteFileRemotely(peer, selectedFile.FileName)
					statusLabel.SetText("🗑️ Solicitud enviada para eliminar archivo remoto.")
					break
				}
			}
		}
	})

	transferButton := widget.NewButtonWithIcon("Transferir", theme.MailForwardIcon(), func() {
		if selectedFile == nil {
			statusLabel.SetText("❌ Selecciona un archivo para transferir.")
			return
		}
		if selectedFile.PeerID != localID {
			for _, peer := range peerSystem.Peers {
				if peer.ID == selectedFile.PeerID {
					go requestFileFromPeer(peer, selectedFile.FileName)
					statusLabel.SetText("⬇️ Archivo solicitado desde máquina remota.")
					break
				}
			}
			return
		}

		count := 0
		for id, chk := range peerCheckMap {
			if chk.Checked {
				for _, p := range peerSystem.Peers {
					if p.ID == id {
						go sendFileToPeer(p, selectedFile.FileName)
						count++
						break
					}
				}
			}
		}
		if count > 0 {
			statusLabel.SetText(fmt.Sprintf("📤 Archivo enviado a %d máquina(s).", count))
		} else {
			statusLabel.SetText("⚠️ No se seleccionó ninguna máquina.")
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
		label += fmt.Sprintf(" - %s:%s", pinfo.IP, pinfo.Port)

		title := canvas.NewText(label, color.White)
		title.TextStyle = fyne.TextStyle{Bold: true}
		title.Alignment = fyne.TextAlignCenter

		state := widget.NewLabel("🔴 Offline")
		machineStates[pinfo.ID] = state

		fileList := container.NewVBox()
		machineFileLists[pinfo.ID] = fileList

		content := container.NewVBox(title, state, widget.NewSeparator(), fileList)
		border := canvas.NewRectangle(colors[i%len(colors)])
		border.StrokeWidth = 4
		border.StrokeColor = colors[i%len(colors)]
		border.FillColor = color.NRGBA{R: 20, G: 20, B: 20, A: 255}
		border.SetMinSize(fyne.NewSize(500, 250))

		panel := container.NewMax(border, container.NewPadded(content))
		machinePanels[pinfo.ID] = panel
		grid.Add(panel)
	}

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		offlineMap := make(map[int]bool)

		for range ticker.C {
			syncIcon.SetText("🔄 Sincronizando...")

			for _, pinfo := range peerSystem.Peers {
				files, err := fs.GetFilesByPeer(pinfo, localID)
				isOnline := err == nil
				wasOffline := offlineMap[pinfo.ID]
				offlineMap[pinfo.ID] = !isOnline

				if isOnline {
					machineStates[pinfo.ID].SetText("🟢 En línea")
				} else {
					machineStates[pinfo.ID].SetText("🔴 Offline")
				}

				if isOnline && wasOffline && pinfo.ID != localID {
					go sendLogsToPeer(pinfo)
					statusLabel.SetText(fmt.Sprintf("📤 Logs enviados a Maq%d tras reconexión", pinfo.ID))
				}

				machineFileLists[pinfo.ID].Objects = nil
				if isOnline {
					for _, file := range files {
						name := file.Name
						mod := file.ModTime.Format("02-Jan 15:04")
						icon := getIconForFile(name)
						btn := widget.NewButtonWithIcon(fmt.Sprintf("%s (%s)", name, mod), icon, nil)
						btn.Alignment = widget.ButtonAlignLeading
						btn.Importance = widget.MediumImportance
						pid := pinfo.ID
						fname := name
						thisBtn := btn
						var lastClick time.Time

						btn.OnTapped = func() {
							now := time.Now()
							if selectedButton != nil {
								(*selectedButton).Importance = widget.MediumImportance
								(*selectedButton).Refresh()
							}
							selectedFile = &SelectedFile{FileName: fname, PeerID: pid}
							selectedButton = thisBtn
							thisBtn.Importance = widget.HighImportance
							thisBtn.Refresh()
							selectedLabel.SetText("Archivo seleccionado: " + fname + " (Maq" + strconv.Itoa(pid) + ")")
							if pid == localID && now.Sub(lastClick) < 500*time.Millisecond {
								go openFile(fname)
							}
							lastClick = now
						}
						machineFileLists[pinfo.ID].Add(btn)
					}
				}
				machineFileLists[pinfo.ID].Refresh()
			}
			syncIcon.SetText("✅ Actualizado")
		}
	}()

	myApp.Run()
}



func sendLogsToPeer(pinfo peer.PeerInfo) {
    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", pinfo.IP, pinfo.Port))
    if err != nil {
        fmt.Println("❌ No se pudo conectar a", pinfo.IP, "para enviar logs.")
        return
    }
    defer conn.Close()

    logs := logger.GetLogs()
    msg := map[string]interface{}{
        "type": "SYNC_LOGS",
        "logs": logs,
    }

    err = json.NewEncoder(conn).Encode(msg)
    if err != nil {
        fmt.Println("❌ Error al enviar logs:", err)
    } else {
        fmt.Println("📤 Logs enviados a Maq", pinfo.ID)
    }
}


// ✅ Diálogo para seleccionar peers con opción "Todas las máquinas"
func showPeerSelectionDialog(parent fyne.Window, ps *peer.Peer, filename string, statusLabel *widget.Label) {
    var checkboxes []*widget.Check
    selectedPeers := make(map[int]bool)

    for _, p := range ps.Peers {
        if p.ID == ps.Local.ID {
            continue
        }
        pid := p.ID
        selectedPeers[pid] = false
        label := fmt.Sprintf("Máquina %d (%s:%s)", pid, p.IP, p.Port)
        chk := widget.NewCheck(label, func(checked bool) {
            selectedPeers[pid] = checked
        })
        checkboxes = append(checkboxes, chk)
    }

    selectAll := widget.NewCheck("Todas las máquinas", func(checked bool) {
        for pid := range selectedPeers {
            selectedPeers[pid] = checked
        }
        for _, cb := range checkboxes {
            cb.SetChecked(checked)
        }
    })

    confirm := widget.NewButton("Enviar", func() {
        count := 0
        for _, p := range ps.Peers {
            if selectedPeers[p.ID] {
                go sendFileToPeer(p, filename)
                count++
            }
        }
        if count > 0 {
            statusLabel.SetText(fmt.Sprintf("📤 Archivo enviado a %d máquina(s).", count))
        } else {
            statusLabel.SetText("⚠️ No se seleccionó ninguna máquina.")
        }
        parent.Close()
    })

    var objs []fyne.CanvasObject
    objs = append(objs, selectAll)
    for _, c := range checkboxes {
        objs = append(objs, c)
    }
    objs = append(objs, confirm)

    dialogContent := container.NewVBox(objs...)
    
	dlg := dialog.NewCustom("Seleccionar destino", "Cancelar", dialogContent, parent)
	dlg.Resize(fyne.NewSize(400, 300))
	dlg.Show()
}

// Abre un archivo local con el programa predeterminado
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
    err := cmd.Start()
    if err != nil {
        fmt.Println("❌ Error al abrir el archivo:", err)
    }
}

// Devuelve un ícono según extensión del archivo
func getIconForFile(name string) fyne.Resource {
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

// Envía archivo local a otro nodo
func sendFileToPeer(peer peer.PeerInfo, filename string) {
    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", peer.IP, peer.Port))
    if err != nil {
        fmt.Println("❌ No se pudo conectar a", peer.IP)
        return
    }
    defer conn.Close()

    data, err := os.ReadFile(filepath.Join("shared", filename))
    if err != nil {
        fmt.Println("❌ No se pudo leer el archivo:", err)
        return
    }

    msg := map[string]interface{}{
        "type":    "SEND_FILE",
        "name":    filename,
        "content": base64.StdEncoding.EncodeToString(data),
    }
    json.NewEncoder(conn).Encode(msg)
}

// Solicita archivo remoto a otro nodo
func requestFileFromPeer(peer peer.PeerInfo, filename string) {
    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", peer.IP, peer.Port))
    if err != nil {
        fmt.Println("❌ No se pudo conectar a", peer.IP)
        return
    }
    defer conn.Close()

    req := map[string]interface{}{
        "type": "GET_FILE",
        "name": filename,
    }
    json.NewEncoder(conn).Encode(req)

    var resp map[string]interface{}
    err = json.NewDecoder(conn).Decode(&resp)
    if err != nil || resp["type"] != "FILE_CONTENT" {
        fmt.Println("❌ Error al recibir archivo:", err)
        return
    }

    decoded, _ := base64.StdEncoding.DecodeString(resp["content"].(string))
    os.WriteFile(filepath.Join("shared", filename), decoded, 0644)
}

// Elimina archivo remoto
func deleteFileRemotely(peer peer.PeerInfo, filename string) {
    conn, err := net.Dial("tcp", fmt.Sprintf("%s:%s", peer.IP, peer.Port))
    if err != nil {
        fmt.Println("❌ No se pudo conectar a", peer.IP)
        return
    }
    defer conn.Close()

    req := map[string]interface{}{
        "type": "DELETE_FILE",
        "name": filename,
    }
    json.NewEncoder(conn).Encode(req)
}
