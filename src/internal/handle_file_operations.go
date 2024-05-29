package internal

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/lithammer/shortuuid"
	"github.com/yorukot/superfile/src/config/icon"
)

// Create a file in the currently focus file panel
func panelCreateNewFile(m model) model {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	ti := textinput.New()
	ti.Cursor.Style = modalCursorStyle
	ti.Cursor.TextStyle = modalStyle
	ti.TextStyle = modalStyle
	ti.Cursor.Blink = true
	ti.Placeholder = "Add \"/\" represent Transcend folder at the end"
	ti.PlaceholderStyle = modalStyle
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = modalWidth - 10

	m.typingModal.location = panel.location
	m.typingModal.open = true
	m.typingModal.textInput = ti
	m.firstTextInput = true

	m.fileModel.filePanels[m.filePanelFocusIndex] = panel

	return m
}

// Rename file where the cusror is located
func panelItemRename(m model) model {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	if len(panel.element) == 0 {
		return m
	}
	ti := textinput.New()
	ti.Cursor.Style = filePanelCursorStyle
	ti.Cursor.TextStyle = filePanelStyle
	ti.TextStyle = modalStyle
	ti.Cursor.Blink = true
	ti.Placeholder = "New name"
	ti.PlaceholderStyle = modalStyle
	ti.SetValue(panel.element[panel.cursor].name)
	ti.Focus()
	ti.CharLimit = 156
	ti.Width = m.fileModel.width - 4

	m.fileModel.renaming = true
	panel.renaming = true
	m.firstTextInput = true
	panel.rename = ti
	m.fileModel.filePanels[m.filePanelFocusIndex] = panel
	return m
}

func deleteItemWarn(m model) model {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	if !((panel.panelMode == selectMode && len(panel.selected) != 0) || (panel.panelMode == browserMode)) {
		return m
	}
	id := shortuuid.New()
	message := channelMessage{
		messageId:       id,
		messageType:  snedWarnModal,
	}

	if isExternalDiskPath(panel.location) {
		message.warnModal = warnModal{
			open:     true,
			title:    "Are you sure you want to completely delete",
			content:  "This operation cannot be undone and your data will be completely lost.",
			warnType: confirmDeleteItem,
		}
		channel <- message
		return m
	} else {
		message.warnModal = warnModal{
			open:     true,
			title:    "Are you sure you want to move this to trash can",
			content:  "This operation will move file or directory to trash can.",
			warnType: confirmDeleteItem,
		}
		channel <- message
		return m
	}
}

// Move file or directory to the trash can
func deleteSingleItem(m model) model {
	id := shortuuid.New()
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]

	if len(panel.element) == 0 {
		return m
	}

	prog := progress.New(generateGradientColor())
	prog.PercentageStyle = footerStyle

	newProcess := process{
		name:     icon.Delete + icon.Space + panel.element[panel.cursor].name,
		progress: prog,
		state:    inOperation,
		total:    1,
		done:     0,
	}
	m.processBarModel.process[id] = newProcess

	message := channelMessage{
		messageId: id,
		messageType: sendProcess,
		processNewState: newProcess,
	}

	channel <- message
	err := trashMacOrLinux(panel.element[panel.cursor].location)

	if err != nil {
		p := m.processBarModel.process[id]
		p.state = failure
		message.processNewState = p
		channel <- message
	} else {
		p := m.processBarModel.process[id]
		p.done = 1
		p.state = successful
		p.doneTime = time.Now()
		message.processNewState = p
		channel <- message
	}
	if panel.cursor == len(panel.element)-1 {
		panel.cursor--
	}
	m.fileModel.filePanels[m.filePanelFocusIndex] = panel
	return m
}

// Move file or directory to the trash can
func deleteMultipleItems(m model) {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	if len(panel.selected) != 0 {
		id := shortuuid.New()
		prog := progress.New(generateGradientColor())
		prog.PercentageStyle = footerStyle

		newProcess := process{
			name:     icon.Delete + icon.Space + filepath.Base(panel.selected[0]),
			progress: prog,
			state:    inOperation,
			total:    len(panel.selected),
			done:     0,
		}

		m.processBarModel.process[id] = newProcess

		message := channelMessage{
			messageId: id,
			messageType: sendProcess,
			processNewState: newProcess,
		}

		channel <- message

		for _, filePath := range panel.selected {

			p := m.processBarModel.process[id]
			p.name = icon.Delete + icon.Space + filepath.Base(filePath)
			p.done++
			p.state = inOperation
			if len(channel) < 5 {
				message.processNewState = p
				channel <- message
			}
			err := trashMacOrLinux(filePath)

			if err != nil {
				p.state = failure
				message.processNewState = p
				channel <- message
				outPutLog("Delete multiple item function error", err)
				m.processBarModel.process[id] = p
				break
			} else {
				if p.done == p.total {
					p.state = successful
					message.processNewState = p
					channel <- message
				}
				m.processBarModel.process[id] = p
			}
		}
	}

	if panel.cursor >= len(panel.element)-len(panel.selected)-1 {
		panel.cursor = len(panel.element) - len(panel.selected) - 1
		if panel.cursor < 0 {
			panel.cursor = 0
		}
	}
	panel.selected = panel.selected[:0]
}

// Completely delete file or folder (Not move to the trash can)
func completelyDeleteSingleItem(m model) model {
	id := shortuuid.New()
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]

	if len(panel.element) == 0 {
		return m
	}

	prog := progress.New(generateGradientColor())
	prog.PercentageStyle = footerStyle

	newProcess := process{
		name:     "󰆴 " + panel.element[panel.cursor].name,
		progress: prog,
		state:    inOperation,
		total:    1,
		done:     0,
	}
	m.processBarModel.process[id] = newProcess

	message := channelMessage{
		messageId: id,
		messageType: sendProcess,
		processNewState: newProcess,
	}
	
	channel <- message

	err := os.RemoveAll(panel.element[panel.cursor].location)
	if err != nil {
		outPutLog("Completely delete single item function remove file error", err)
	}

	if err != nil {
		p := m.processBarModel.process[id]
		p.state = failure
		message.processNewState = p
		channel <- message
	} else {
		p := m.processBarModel.process[id]
		p.done = 1
		p.state = successful
		p.doneTime = time.Now()
		message.processNewState = p
		channel <- message
	}
	if panel.cursor == len(panel.element)-1 {
		panel.cursor--
	}
	m.fileModel.filePanels[m.filePanelFocusIndex] = panel
	return m
}

// Completely delete all file or folder from clipboard (Not move to the trash can)
func completelyDeleteMultipleItems(m model) {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	if len(panel.selected) != 0 {
		id := shortuuid.New()
		prog := progress.New(generateGradientColor())
		prog.PercentageStyle = footerStyle

		newProcess := process{
			name:     icon.Delete + icon.Space + filepath.Base(panel.selected[0]),
			progress: prog,
			state:    inOperation,
			total:    len(panel.selected),
			done:     0,
		}

		m.processBarModel.process[id] = newProcess

		message := channelMessage{
			messageId:  id,
			messageType: sendProcess,
			processNewState: newProcess,
		}

		channel <- message
		for _, filePath := range panel.selected {

			p := m.processBarModel.process[id]
			p.name = icon.Delete + icon.Space + filepath.Base(filePath)
			p.done++
			p.state = inOperation
			if len(channel) < 5 {
				message.processNewState = p
				channel <- message
			}
			err := os.RemoveAll(filePath)
			if err != nil {
				outPutLog("Completely delete multiple item function remove file error", err)
			}

			if err != nil {
				p.state = failure
				message.processNewState = p
				channel <- message
				outPutLog("Completely delete multiple item function error", err)
				m.processBarModel.process[id] = p
				break
			} else {
				if p.done == p.total {
					p.state = successful
					message.processNewState = p
					channel <- message
				}
				m.processBarModel.process[id] = p
			}
		}
	}

	if panel.cursor >= len(panel.element)-len(panel.selected)-1 {
		panel.cursor = len(panel.element) - len(panel.selected) - 1
		if panel.cursor < 0 {
			panel.cursor = 0
		}
	}
	panel.selected = panel.selected[:0]
}

// Copy directory or file
func copySingleItem(m model) model {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	m.copyItems.cut = false
	m.copyItems.items = m.copyItems.items[:0]
	if len(panel.element) == 0 {
		return m
	}
	m.copyItems.items = append(m.copyItems.items, panel.element[panel.cursor].location)
	fileInfo, err := os.Stat(panel.element[panel.cursor].location)
	if os.IsNotExist(err) {
		m.copyItems.items = m.copyItems.items[:0]
		return m
	}
	if err != nil {
		outPutLog("Copy single item get file state error", panel.element[panel.cursor].location, err)
	}

	if !fileInfo.IsDir() && float64(fileInfo.Size())/(1024*1024) < 250 {
		fileContent, err := os.ReadFile(panel.element[panel.cursor].location)

		if err != nil {
			outPutLog("Copy single item read file error", panel.element[panel.cursor].location, err)
		}

		if err := clipboard.WriteAll(string(fileContent)); err != nil {
			outPutLog("Copy single item write file error", panel.element[panel.cursor].location, err)
		}
	}
	m.fileModel.filePanels[m.filePanelFocusIndex] = panel
	return m
}

// Copy all selected file or directory to the clipboard
func copyMultipleItem(m model) model {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	m.copyItems.cut = false
	m.copyItems.items = m.copyItems.items[:0]
	if len(panel.selected) == 0 {
		return m
	}
	m.copyItems.items = panel.selected
	fileInfo, err := os.Stat(panel.selected[0])
	if os.IsNotExist(err) {
		return m
	}
	if err != nil {
		outPutLog("Copy multiple item function get file state error", panel.selected[0], err)
	}

	if !fileInfo.IsDir() && float64(fileInfo.Size())/(1024*1024) < 250 {
		fileContent, err := os.ReadFile(panel.selected[0])

		if err != nil {
			outPutLog("Copy multiple item function read file error", err)
		}

		if err := clipboard.WriteAll(string(fileContent)); err != nil {
			outPutLog("Copy multiple item function write file to clipboard error", err)
		}
	}
	m.fileModel.filePanels[m.filePanelFocusIndex] = panel
	return m
}

// Cut directory or file
func cutSingleItem(m model) model {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	m.copyItems.cut = true
	m.copyItems.items = m.copyItems.items[:0]
	if len(panel.element) == 0 {
		return m
	}
	m.copyItems.items = append(m.copyItems.items, panel.element[panel.cursor].location)
	fileInfo, err := os.Stat(panel.element[panel.cursor].location)
	if os.IsNotExist(err) {
		m.copyItems.items = m.copyItems.items[:0]
		return m
	}
	if err != nil {
		outPutLog("Cut single item get file state error", panel.element[panel.cursor].location, err)
	}

	if !fileInfo.IsDir() && float64(fileInfo.Size())/(1024*1024) < 250 {
		fileContent, err := os.ReadFile(panel.element[panel.cursor].location)

		if err != nil {
			outPutLog("Cut single item read file error", panel.element[panel.cursor].location, err)
		}

		if err := clipboard.WriteAll(string(fileContent)); err != nil {
			outPutLog("Cut single item write file error", panel.element[panel.cursor].location, err)
		}
	}
	m.fileModel.filePanels[m.filePanelFocusIndex] = panel
	return m
}

// Cut all selected file or directory to the clipboard
func cutMultipleItem(m model) model {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	m.copyItems.cut = true
	m.copyItems.items = m.copyItems.items[:0]
	if len(panel.selected) == 0 {
		return m
	}
	m.copyItems.items = panel.selected
	fileInfo, err := os.Stat(panel.selected[0])
	if os.IsNotExist(err) {
		return m
	}
	if err != nil {
		outPutLog("Copy multiple item function get file state error", panel.selected[0], err)
	}

	if !fileInfo.IsDir() && float64(fileInfo.Size())/(1024*1024) < 250 {
		fileContent, err := os.ReadFile(panel.selected[0])

		if err != nil {
			outPutLog("Copy multiple item function read file error", err)
		}

		if err := clipboard.WriteAll(string(fileContent)); err != nil {
			outPutLog("Copy multiple item function write file to clipboard error", err)
		}
	}
	m.fileModel.filePanels[m.filePanelFocusIndex] = panel
	return m
}

// Paste all clipboard items
func pasteItem(m model) {
	id := shortuuid.New()
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]

	if len(m.copyItems.items) == 0 {
		return 
	}

	totalFiles := 0

	for _, folderPath := range m.copyItems.items {
		count, err := countFiles(folderPath)
		if err != nil {
			continue
		}
		totalFiles += count
	}

	prog := progress.New(generateGradientColor())
	prog.PercentageStyle = footerStyle

	prefixIcon := icon.Copy + icon.Space
	if m.copyItems.cut {
		prefixIcon = icon.Cut + icon.Space
	}

	newProcess := process{
		name:     prefixIcon + filepath.Base(m.copyItems.items[0]),
		progress: prog,
		state:    inOperation,
		total:    totalFiles,
		done:     0,
	}

	m.processBarModel.process[id] = newProcess

	message := channelMessage{
		messageId: id,
		messageType: sendProcess,
		processNewState: newProcess,
	}

	channel <- message

	p := m.processBarModel.process[id]
	for _, filePath := range m.copyItems.items {
		var err error
		if m.copyItems.cut && !isExternalDiskPath(filePath) {
			p.name = icon.Cut + icon.Space + filepath.Base(filePath)
		} else {
			if m.copyItems.cut {
				p.name = icon.Cut + icon.Space + filepath.Base(filePath)
			}
			p.name = icon.Copy + icon.Space + filepath.Base(filePath)
		}

		errMessage := "cut item error"
		if m.copyItems.cut && !isExternalDiskPath(filePath) {
			err = moveElement(filePath, filepath.Join(panel.location, path.Base(filePath)))
		} else {
			newModel, err := pasteDir(filePath, filepath.Join(panel.location, path.Base(filePath)), id, m)
			if err != nil {
				errMessage = "paste item error"
			}
			m = newModel
			if m.copyItems.cut {
				os.RemoveAll(filePath)
			}
		}
		p = m.processBarModel.process[id]
		if err != nil {
			p.state = failure
			message.processNewState = p
			channel <- message
			outPutLog(errMessage, err)
			m.processBarModel.process[id] = p
			break
		}
	}

	p.state = successful
	p.done = totalFiles
	p.doneTime = time.Now()
	message.processNewState = p
	channel <- message

	m.processBarModel.process[id] = p
	m.copyItems.cut = false
}

// Extrach compress file
func extractFile(m model) {
	var err error
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	ext := strings.ToLower(filepath.Ext(panel.element[panel.cursor].location))
	outputDir := fileNameWithoutExtension(panel.element[panel.cursor].location)
	outputDir, err = renameIfDuplicate(outputDir)

	if err != nil {
		outPutLog("Error extract file when create new directory", err)
	}

	switch ext {
	case ".zip":
		os.MkdirAll(outputDir, 0755)
		err = unzip(panel.element[panel.cursor].location, outputDir)
		if err != nil {
			outPutLog("Error extract file,", err)
		}
	default:
		os.MkdirAll(outputDir, 0755)
		err = extractCompressFile(panel.element[panel.cursor].location, outputDir)
		if err != nil {
			outPutLog("Error extract file,", err)
		}
	}
}

// Compress file or directory
func compressFile(m model) {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]
	fileName := filepath.Base(panel.element[panel.cursor].location)

	zipName := strings.TrimSuffix(fileName, filepath.Ext(fileName)) + ".zip"
	zipName, err := renameIfDuplicate(zipName)

	if err != nil {
		outPutLog("Error compress file when rename duplicate", err)
	}

	zipSource(panel.element[panel.cursor].location, filepath.Join(filepath.Dir(panel.element[panel.cursor].location), zipName))
}

// Open file with default editor
func openFileWithEditor(m model) tea.Cmd {
	panel := m.fileModel.filePanels[m.filePanelFocusIndex]

	editor := os.Getenv("EDITOR")
	m.editorMode = true
	if editor == "" {
		editor = "nano"
	}
	c := exec.Command(editor, panel.element[panel.cursor].location)

	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err}
	})
}

// Open directory with default editor
func openDirectoryWithEditor(m model) tea.Cmd {
	editor := os.Getenv("EDITOR")
	m.editorMode = true
	if editor == "" {
		editor = "nano"
	}
	c := exec.Command(editor, m.fileModel.filePanels[m.filePanelFocusIndex].location)
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return editorFinishedMsg{err}
	})
}
