package gui

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"image/color"

	"p2pfs/internal/fs"
	"p2pfs/internal/peer"
)

type SelectedFile struct {
	FileName string
	PeerID   int
}

func Run() {
	myApp := app.New()
	myWindow := myApp.NewWindow("Sistema Distribuido P2P")
	myWindow.Resize(fyne.NewSize(1200, 700))

	statusLabel := widget.NewLabel("Cargando archivos...")
	selectedLabel := widget.NewLabel("Archivo seleccionado: ninguno")

	grid := container.NewGridWithColumns(2)
	scroll := container.NewVScroll(grid)
	scroll.SetMinSize(fyne.NewSize(1000, 600))

	var selectedFile *SelectedFile
	var selectedButton *widget.Button

	btnEliminar := widget.NewButtonWithIcon("Eliminar", theme.DeleteIcon(), func() {
		if selectedFile != nil {
			fmt.Printf("🗑️ Eliminar archivo %s de Maq%d\n", selectedFile.FileName, selectedFile.PeerID)
		}
	})
	btnTransferir := widget.NewButtonWithIcon("Transferir", theme.MailForwardIcon(), func() {
		if selectedFile != nil {
			fmt.Printf("📤 Transferir archivo %s desde Maq%d\n", selectedFile.FileName, selectedFile.PeerID)
		}
	})
	btnActualizar := widget.NewButtonWithIcon("Actualizar", theme.ViewRefreshIcon(), func() {
		grid.Objects = nil
		grid.Refresh()
		go loadMachines(grid, statusLabel, selectedLabel, &selectedFile, &selectedButton)
	})

	header := container.NewVBox(
		canvas.NewText("Sistema Distribuido P2P", theme.ForegroundColor()),
		container.NewHBox(btnEliminar, btnTransferir, btnActualizar, layout.NewSpacer(), selectedLabel),
		statusLabel,
	)

	myWindow.SetContent(container.NewBorder(header, nil, nil, nil, scroll))
	myWindow.Show()

	go loadMachines(grid, statusLabel, selectedLabel, &selectedFile, &selectedButton)

	myApp.Run()
}

func loadMachines(
	grid *fyne.Container,
	statusLabel *widget.Label,
	selectedLabel *widget.Label,
	selectedFile **SelectedFile,
	selectedButton **widget.Button,
) {
	peerSystem := peer.InitPeer()
	if peerSystem == nil {
		statusLabel.SetText("❌ Error al cargar peers.json")
		return
	}

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

		content := container.NewVBox(
			title,
			state,
			widget.NewSeparator(),
			container.NewVBox(fileWidgets...),
		)

		// Panel con borde de color
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
