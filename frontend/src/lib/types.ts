export type StoragePartitionFilter = 'all' | 'root' | 'home';

export interface MountStats {
  path: string;
  total_bytes: number;
  free_bytes: number;
  used_bytes: number;
  used_percent: number;
}

export interface SystemStorageInfo {
  root: MountStats;
  home: MountStats;
  has_separate_home: boolean;
}

export interface DistroInfo {
  id: string;
  id_like?: string[];
  name: string;
  pretty_name: string;
  version_id?: string;
  family: string;
  raw?: Record<string, string>;
}

export interface ModuleSummary {
  id: string;
  name: string;
  category: string;
  description: string;
  available: boolean;
  requires_elevation: boolean;
  enabled: boolean;
}

export interface CleanableItem {
  id: string;
  name: string;
  description: string;
  path?: string;
  size: number;
  selected: boolean;
  safety: 'SAFE' | 'CAUTION' | 'HIGH' | string;
  mod_time?: string | null;
  metadata?: Record<string, string>;
}

export interface ScanResult {
  items: CleanableItem[];
  total_size: number;
  scanned_at?: string;
  error?: string | null;
  metadata?: Record<string, string>;
}

export interface CleanResult {
  cleaned_items: CleanableItem[];
  freed_bytes: number;
  errors?: string[];
  duration_ms?: number;
  success?: boolean;
}

export interface AppConfig {
  version?: string;
  general: {
    dry_run_default: boolean;
    snapshot_safeguard: boolean;
    log_level: string;
  };
  retention: {
    snapper_min_age_days: number;
    pacman_keep_versions: number;
    journal_vacuum_days: number;
    dev_cache_idle_days: number;
  };
  modules?: Record<string, { enabled: boolean }>;
  custom_rules?: CustomScanRule[];
}

export interface CustomScanRule {
  id: string;
  name: string;
  path: string;
  pattern: string;
  recursive: boolean;
  min_age_days: number;
  safety: string;
}

export interface ExecutionLogEntry {
  id: string;
  timestamp: string;
  level: 'INIT' | 'SNAP' | 'CLEAN' | 'WARN' | 'ERROR' | 'DONE';
  message: string;
}
