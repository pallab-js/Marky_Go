import { AppInfo, CreateFolder, CreateNote, DeletePath, GetSettings, GetTree, Health, OpenVault, OpenVaultDialog, ReadNote, RebuildIndex, RenamePath, SaveConflictCopy, Search, UpdateSettings, WriteNote } from '../../wailsjs/go/app/App';
import type { domain } from '../../wailsjs/go/models';

export type Settings = domain.Settings;
export type TreeNode = domain.TreeNode;
export type NoteDocument = domain.NoteDocument;
export type VaultInfo = domain.VaultInfo;
export type SearchResult = domain.SearchResult;
export type SearchOptions = domain.SearchOptions;
export type SaveResult = domain.SaveResult;

export const api = {
  appInfo: AppInfo,
  getSettings: GetSettings,
  updateSettings: UpdateSettings,
  health: Health,
  openVaultDialog: OpenVaultDialog,
  openVault: OpenVault,
  getTree: GetTree,
  readNote: ReadNote,
  rebuildIndex: RebuildIndex,
  search: Search,
  writeNote: WriteNote,
  saveConflictCopy: SaveConflictCopy,
  createNote: CreateNote,
  createFolder: CreateFolder,
  renamePath: RenamePath,
  deletePath: DeletePath,
};



