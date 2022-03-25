/* SPDX-License-Identifier: Apache-2.0 */
/* Copyright Contributors to the cpackget project. */

package installer

import (
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	errs "github.com/open-cmsis-pack/cpackget/cmd/errors"
	"github.com/open-cmsis-pack/cpackget/cmd/ui"
	"github.com/open-cmsis-pack/cpackget/cmd/utils"
	"github.com/open-cmsis-pack/cpackget/cmd/xml"
	log "github.com/sirupsen/logrus"
)

// AddPack adds a pack to the pack installation directory structure
func AddPack(packPath string, checkEula, extractEula bool) error {

	pack, err := preparePack(packPath)
	if err != nil {
		return err
	}

	log.Infof("Adding pack \"%s.%s.%s\"", pack.Vendor, pack.Name, pack.Version)

	if !extractEula && pack.isInstalled {
		log.Errorf("pack %s.%s.%s is already installed here: %s", pack.Vendor, pack.Name, pack.Version, filepath.Join(Installation.PackRoot, pack.Vendor, pack.Name, pack.Version))
		return errs.ErrAlreadyLogged
	}

	if pack.isPackID {
		pack.path, err = FindPackURL(pack)
		if err != nil {
			return err
		}
	}

	if err = pack.fetch(); err != nil {
		return err
	}

	// Tells the UI to return right away with the [E]xtract option selected
	ui.Extract = extractEula

	if err = pack.install(Installation, checkEula || extractEula); err != nil {
		return err
	}

	return Installation.touchPackIdx()
}

// RemovePack removes a pack given a pack path
func RemovePack(packPath string, purge bool) error {
	log.Debugf("Removing pack \"%v\"", packPath)

	// TODO: by default, remove latest version first
	// if no version is given

	pack, err := preparePack(packPath)
	if err != nil {
		return err
	}

	if pack.isInstalled {
		// TODO: If removing-all is enabled, get rid of the version
		// pack.Version = ""
		if err = pack.uninstall(Installation); err != nil {
			return err
		}

		if purge {
			if err = pack.purge(); err != nil {
				return err
			}
		}

		return Installation.touchPackIdx()
	} else if purge {
		return pack.purge()
	}

	log.Errorf("Pack \"%v\" is not installed", packPath)
	return errs.ErrPackNotInstalled
}

// AddPdsc adds a pack via PDSC file
func AddPdsc(pdscPath string) error {
	log.Debugf("Adding pdsc \"%v\"", pdscPath)

	pdsc, err := preparePdsc(pdscPath)
	if err != nil {
		return err
	}

	if err := pdsc.install(Installation); err != nil {
		return err
	}

	if err := Installation.LocalPidx.Write(); err != nil {
		return err
	}

	return Installation.touchPackIdx()
}

// RemovePdsc removes a pack given a pdsc path
func RemovePdsc(pdscPath string) error {
	log.Debugf("Removing pdsc \"%v\"", pdscPath)

	pdsc, err := preparePdsc(pdscPath)
	if err != nil {
		return err
	}

	if err = pdsc.uninstall(Installation); err != nil {
		return err
	}

	if err := Installation.LocalPidx.Write(); err != nil {
		return err
	}

	return Installation.touchPackIdx()
}

// UpdatePublicIndex receives a index path and place it under .Web/index.pidx.
func UpdatePublicIndex(indexPath string, overwrite bool) error {
	log.Debugf("Updating public index with \"%v\"", indexPath)

	if utils.FileExists(Installation.PublicIndex) {
		if !overwrite {
			return errs.ErrCannotOverwritePublicIndex
		}
		log.Infof("Overwriting public index file %v", Installation.PublicIndex)
	}

	var err error
	if !strings.HasPrefix(indexPath, "https://") {
		return errs.ErrIndexPathNotSafe
	}

	indexPath, err = utils.DownloadFile(indexPath)
	if err != nil {
		return err
	}

	pidx := xml.NewPidxXML(indexPath)
	if err := pidx.Read(); err != nil {
		_ = os.Remove(indexPath)
		return err
	}

	return utils.MoveFile(indexPath, filepath.Join(Installation.WebDir, "index.pidx"))
}

// ListInstalledPacks generates a list of all packs present in the pack root folder
func ListInstalledPacks(listCached, listPublic bool) error {
	log.Debugf("Listing packs")
	if listPublic {
		log.Info("Listing packs from the public index")
		pdscMap := Installation.PublicIndexXML.ListPdscs()

		// Sort by keys
		keys := make([]string, len(pdscMap))
		i := 0
		for k := range pdscMap {
			keys[i] = k
			i++
		}

		if i == 0 {
			log.Info("(no packs in public index)")
			return nil
		}

		sort.Slice(keys, func(i, j int) bool {
			return strings.ToLower(keys[i]) < strings.ToLower(keys[j])
		})
		// List all available packs from the index
		for _, pdscKey := range keys {
			pdscTag := pdscMap[pdscKey]
			logMessage := pdscKey
			packFilePath := filepath.Join(Installation.DownloadDir, pdscTag.Key()) + ".pack"

			if Installation.PackIsInstalled(&PackType{PdscTag: pdscTag}) {
				logMessage += " (installed)"
			} else if utils.FileExists(packFilePath) {
				logMessage += " (cached)"
			}
			log.Info(logMessage)
		}
	} else if listCached {
		log.Info("Listing cached packs")
		pattern := filepath.Join(Installation.DownloadDir, "*.pack")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}

		if len(matches) == 0 {
			log.Info("(no packs cached)")
			return nil
		}

		sort.Slice(matches, func(i, j int) bool {
			return strings.ToLower(matches[i]) < strings.ToLower(matches[j])
		})
		for _, packFilePath := range matches {
			packInfo, err := utils.ExtractPackInfo(packFilePath)
			if err != nil {
				log.Errorf("A pack in the cache folder has malformed pack name: %s", packFilePath)
				return errs.ErrUnknownBehavior
			}

			pdscTag := xml.PdscTag{
				Vendor:  packInfo.Vendor,
				Name:    packInfo.Pack,
				Version: packInfo.Version,
			}

			logMessage := pdscTag.Key()
			if Installation.PackIsInstalled(&PackType{PdscTag: pdscTag}) {
				logMessage += " (installed)"
			}

			log.Info(logMessage)
		}
	} else {
		log.Info("Listing installed packs")
		pattern := filepath.Join(Installation.PackRoot, "*", "*", "*", "*.pdsc")
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}

		if len(matches) == 0 {
			log.Info("(no packs installed)")
			return nil
		}

		sort.Slice(matches, func(i, j int) bool {
			return strings.ToLower(matches[i]) < strings.ToLower(matches[j])
		})
		for _, pdscFilePath := range matches {
			log.Debug(pdscFilePath)

			// Transform pdscFilePath into packName
			pdscFilePath = strings.Replace(pdscFilePath, Installation.PackRoot, "", -1)
			packName, _ := filepath.Split(pdscFilePath)
			packName = strings.Replace(packName, "/", " ", -1)
			packName = strings.Replace(packName, "\\", " ", -1)
			packName = strings.Trim(packName, " ")
			packName = strings.Replace(packName, " ", ".", -1) + ".pack"

			packInfo, err := utils.ExtractPackInfo(packName)
			if err != nil {
				log.Errorf("A pack in the cache folder has malformed pack name: %s", packName)
				return errs.ErrUnknownBehavior
			}

			pdscTag := xml.PdscTag{
				Vendor:  packInfo.Vendor,
				Name:    packInfo.Pack,
				Version: packInfo.Version,
			}

			log.Info(pdscTag.Key())
		}
	}

	return nil
}

// FindPackURL uses pack.path as packID and try to find the pack URL
// Finding step are as follows:
// 1. Find pack.Vendor, pack.Name, pack.Version in Installation.PublicIndex
// 1.1. if pack.IsPublic == true
// 1.1.1. read .Web/PDSC file into pdscXML
// 1.1.2. releastTag = pdscXML.FindReleaseTagByVersion(pack.Version)
// 1.1.3. if releaseTag.URL != "", return releaseTag.URL
// 1.1.4. return pdscTag.URL + pack.Vendor + "." + pack.Name + "." + pack.Version + ".pack"
// 1.2. if pack.IsPublic == false
// 1.2.1. if pack's pdsc file not found in Installation.LocalDir then raise errs.ErrPackURLCannotBeFound
// 1.2.2. read .Local/PDSC file into pdscXML
// 1.2.3. releastTag = pdscXML.FindReleaseTagByVersion(pack.Version)
// 1.2.3. if releaseTag == nil then raise ErrPackVersionNotFoundInPdsc
// 1.2.4. if releaseTag.URL != "", return releaseTag.URL
// 1.2.5. return pdscTag.URL + pack.Vendor + "." + pack.Name + "." + pack.Version + ".pack"
func FindPackURL(pack *PackType) (string, error) {
	log.Debugf("Finding URL for \"%v\"", pack.path)

	if pack.IsPublic {
		packPdscFileName := filepath.Join(Installation.WebDir, pack.PdscFileName())
		packPdscXML := xml.NewPdscXML(packPdscFileName)
		if err := packPdscXML.Read(); err != nil {
			return "", err
		}

		// The releaseTag should've been checked before this point
		releaseTag := packPdscXML.FindReleaseTagByVersion(pack.Version)
		if releaseTag == nil {
			return "", errs.ErrUnknownBehavior
		}
		if releaseTag.URL != "" {
			return releaseTag.URL, nil
		}

		return packPdscXML.PackURL(pack.Version), nil
	}

	// if pack.IsPublic == false, it doesn't mean yet it's an actual non-Public pack, need to check in .Local
	packPdscFileName := filepath.Join(Installation.LocalDir, pack.PdscFileName())
	if !utils.FileExists(packPdscFileName) {
		return "", errs.ErrPackURLCannotBeFound
	}

	packPdscXML := xml.NewPdscXML(packPdscFileName)
	if err := packPdscXML.Read(); err != nil {
		return "", err
	}

	releaseTag := packPdscXML.FindReleaseTagByVersion(pack.Version)
	if releaseTag == nil {
		return "", errs.ErrPackVersionNotFoundInPdsc
	}

	pack.Version = releaseTag.Version

	if releaseTag.URL != "" {
		return releaseTag.URL, nil
	}

	return packPdscXML.PackURL(pack.Version), nil
}

// Installation is a singleton variable that keeps the only reference
// to PacksInstallationType
var Installation *PacksInstallationType

// SetPackRoot sets the working directory of the packs installation
// if create == true, cpackget will try to create needed resources
func SetPackRoot(packRoot string, create bool) error {
	if len(packRoot) == 0 {
		log.Infof("Using pack root: \"%v\"", packRoot)
		return errs.ErrPackRootNotFound
	}

	packRoot = filepath.Clean(packRoot)
	log.Infof("Using pack root: \"%v\"", packRoot)

	if !utils.DirExists(packRoot) && !create {
		return errs.ErrPackRootDoesNotExist
	}

	Installation = &PacksInstallationType{
		PackRoot:    packRoot,
		DownloadDir: filepath.Join(packRoot, ".Download"),
		LocalDir:    filepath.Join(packRoot, ".Local"),
		WebDir:      filepath.Join(packRoot, ".Web"),
	}
	Installation.LocalPidx = xml.NewPidxXML(filepath.Join(Installation.LocalDir, "local_repository.pidx"))
	Installation.PackIdx = filepath.Join(packRoot, "pack.idx")
	Installation.PublicIndex = filepath.Join(Installation.WebDir, "index.pidx")
	Installation.PublicIndexXML = xml.NewPidxXML(Installation.PublicIndex)

	for _, dir := range []string{packRoot, Installation.DownloadDir, Installation.LocalDir, Installation.WebDir} {
		log.Debugf("Making sure \"%v\" exists", dir)
		exists := utils.DirExists(dir)
		if !exists {
			if !create {
				return errs.ErrDirectoryNotFound
			} else {
				if err := utils.EnsureDir(dir); err != nil {
					return err
				}
			}
		}
	}

	// Make sure utils.DownloadFile always downloads files to .Download/
	utils.CacheDir = Installation.DownloadDir

	return Installation.PublicIndexXML.Read()
}

// PacksInstallationType is the scruct tha manages Open-CMSIS-Pack installation/deletion.
type PacksInstallationType struct {
	// PackRoot is the working directory if the packs installation
	PackRoot string

	// packs installed
	packs map[string]bool

	// DownloadDir stores copies of all packs that were installed via pack files
	// from external servers.
	DownloadDir string

	// LocalDir stores "local_repository.pidx" containing a list of all packs
	// installed via PDSC files.
	LocalDir string

	// WebDir stores "index.pidx" containing a list of PDSC tags with all
	// publicly available packs.
	WebDir string

	// PublicIndex stores the path PackRoot/WebDir/index.pidx
	PublicIndex string

	// PublicIndexXML stores a xml.PidxXML reference for PackRoot/WebDir/index.pidx
	PublicIndexXML *xml.PidxXML

	// LocalPidx is a reference to "local_repository.pidx" that contains a flat
	// list of PDSC tags representing all packs installed via PDSC files.
	LocalPidx *xml.PidxXML

	// localIsLoaded is a flag that tells whether the local_repository.pidx has been loaded or not
	localIsLoaded bool

	// PackIdx is the "pack.idx" file used by other tools to be notified that
	// the pack installation had changed.
	PackIdx string
}

// touchPackIdx changes the timestamp of pack.idx.
func (p *PacksInstallationType) touchPackIdx() error {
	return utils.TouchFile(p.PackIdx)
}

// PackIsInstalled checks whether a given pack is already installed or not
func (p *PacksInstallationType) PackIsInstalled(pack *PackType) bool {

	installationDir := filepath.Join(p.PackRoot, pack.Vendor, pack.Name, pack.Version)
	dirExists := utils.DirExists(installationDir)
	if pack.Version == "" {
		return dirExists
	}

	return dirExists && utils.FileExists(filepath.Join(installationDir, pack.PdscFileName()))
}

// packIsPublic checks whether the pack is public or not.
// Being public means a PDSC file is present in ".Web/" folder
func (p *PacksInstallationType) packIsPublic(pack *PackType) (bool, error) {
	// lazyly lists all pdsc files in the ".Web/" folder only once
	if p.packs == nil {
		p.packs = make(map[string]bool)
		files, _ := utils.ListDir(p.WebDir, `^.*\.pdsc$`)
		for _, file := range files {
			_, baseFileName := filepath.Split(file)
			p.packs[baseFileName] = true
		}
	}

	_, ok := p.packs[pack.PdscFileName()]
	if ok {
		log.Debugf("Found \"%s\" in \"%s\"", pack.PdscFileName(), p.WebDir)
		return true, nil
	}

	log.Debugf("Not found \"%s\" in \"%s\"", pack.PdscFileName(), p.WebDir)

	// Try to retrieve the packs's PDSC file out of the index.pidx
	pdscTag := p.PublicIndexXML.FindPdscTag(pack.PdscTag)
	if pdscTag == nil {
		log.Debugf("Not found \"%s\" tag in \"%s\"", pack.PdscFileName(), p.PublicIndex)
		return false, nil
	}

	// Download it to .Web/
	pdscFileURL, err := url.Parse(pdscTag.URL)
	if err != nil {
		log.Errorf("Could not parse pdsc url \"%s\": %s", pdscTag.URL, err)
		return false, err
	}
	pdscFileURL.Path = path.Join(pdscFileURL.Path, pack.PdscFileName())
	localFileName, err := utils.DownloadFile(pdscFileURL.String())
	defer os.Remove(localFileName)

	if err != nil {
		log.Errorf("Could not download \"%s\": %s", pdscFileURL, err)
		return false, errs.ErrPackPdscCannotBeFound
	}

	destinationPdscPath := filepath.Join(p.WebDir, pack.PdscFileName())
	err = utils.MoveFile(localFileName, destinationPdscPath)
	if err != nil {
		log.Errorf("Could not move \"%s\" to \"%s\": %s", localFileName, destinationPdscPath, err)
		return false, err
	}

	return true, nil
}
