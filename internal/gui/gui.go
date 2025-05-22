package gui

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
)

type SelectedFile struct {
	FileName string
	PeerID   int
}

func Run(peerSystem *peer.Peer) {
	myApp := app.New()
	myWindow := myApp.NewWindow("Sistema Distribuido P2P")
	myWindow.Resize(fyne.NewSize(1200, 700))

	statusLabel := widget.NewLabel("Cargando archivos...")
	selectedLabel := widget.NewLabel("Archivo seleccionado: ninguno")
	syncIcon := widget.NewLabel("✅ Actualizado")

	grid := container.NewGridWithColumns(2)
	scroll := container.NewVScroll(grid)
	scroll.SetMinSize(fyne.NewSize(1000, 600))

	var selectedFile *SelectedFile
	var selectedButton *widget.Button
	localID := peerSystem.Local.ID

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
		if selectedFile.PeerID == localID {
			for _, peer := range peerSystem.Peers {
				if peer.ID != localID {
					go sendFileToPeer(peer, selectedFile.FileName)
				}
			}
			statusLabel.SetText("📤 Archivo enviado a otras máquinas.")
		} else {
			for _, peer := range peerSystem.Peers {
				if peer.ID == selectedFile.PeerID {
					go requestFileFromPeer(peer, selectedFile.FileName)
					statusLabel.SetText("⬇️ Archivo solicitado desde máquina remota.")
					break
				}
			}
		}
	})

	header := container.NewVBox(
		canvas.NewText("Sistema Distribuido P2P", theme.ForegroundColor()),
		container.NewHBox(deleteButton, transferButton, layout.NewSpacer(), syncIcon),
		container.NewHBox(statusLabel, layout.NewSpacer(), selectedLabel),
	)

	myWindow.SetContent(container.NewBorder(header, nil, nil, nil, scroll))
	myWindow.Show()

	go func() {
		ticker := time.NewTicker(5 * time.Second)
		for range ticker.C {
			syncIcon.SetText("🔄 Sincronizando...")
			grid.Objects = nil
			grid.Refresh()
			loadMachines(peerSystem, grid, statusLabel, selectedLabel, &selectedFile, &selectedButton)
			syncIcon.SetText("✅ Actualizado")
		}
	}()

	go loadMachines(peerSystem, grid, statusLabel, selectedLabel, &selectedFile, &selectedButton)
	myApp.Run()
}

// --- Auxiliares: se mantienen igual ---

func loadMachines(
	peerSystem *peer.Peer,
	grid *fyne.Container,
	statusLabel *widget.Label,
	selectedLabel *widget.Label,
	selectedFile **SelectedFile,
	selectedButton **widget.Button,
) {
	localID := peerSystem.Local.ID
	colors := []color.Color{
		color.NRGBA{R: 180, G: 220, B: 255, A: 255},
		color.NRGBA{R: 200, G: 255, B: 200, A: 255},
		color.NRGBA{R: 255, G: 220, B: 180, A: 255},
		color.NRGBA{R: 255, G: 200, B: 200, A: 255},
	}

	for i, pinfo := range peerSystem.Peers {
		files, err := fs.GetFilesByPeer(pinfo, localID)
		isOnline := err == nil

		title := canvas.NewText(fmt.Sprintf("Maq%d - %s:%s", pinfo.ID, pinfo.IP, pinfo.Port), nil)
		title.TextStyle = fyne.TextStyle{Bold: true}
		title.Alignment = fyne.TextAlignCenter

		state := widget.NewLabel("🔴 Offline")
		if isOnline {
			state.SetText("🟢 En línea")
		}

		var fileWidgets []fyne.CanvasObject
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

				btn.OnTapped = func() {
					if *selectedButton != nil {
						(*selectedButton).Importance = widget.MediumImportance
						(*selectedButton).Refresh()
					}
					*selectedFile = &SelectedFile{FileName: fname, PeerID: pid}
					*selectedButton = thisBtn

					thisBtn.Importance = widget.HighImportance
					thisBtn.Refresh()
					selectedLabel.SetText("Archivo seleccionado: " + fname + " (Maq" + strconv.Itoa(pid) + ")")
				}

				fileWidgets = append(fileWidgets, btn)
			}
		} else {
			fileWidgets = append(fileWidgets, widget.NewLabel("❌ No disponible"))
		}

		content := container.NewVBox(title, state, widget.NewSeparator(), container.NewVBox(fileWidgets...))
		border := canvas.NewRectangle(colors[i%len(colors)])
		border.StrokeWidth = 4
		border.StrokeColor = colors[i%len(colors)]
		border.FillColor = color.NRGBA{R: 255, G: 255, B: 255, A: 255}
		border.SetMinSize(fyne.NewSize(500, 250))

		panel := container.NewMax(border, container.NewPadded(content))
		grid.Add(panel)
		grid.Refresh()
	}
	statusLabel.SetText("✅ Carga completa.")
}

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
