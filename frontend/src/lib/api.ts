import type {
  SystemStorageInfo,
  DistroInfo,
  ModuleSummary,
  ScanResult,
  CleanResult,
  CleanableItem,
} from './types';

// Detect whether running in Wails desktop runtime
export function isWailsRuntime(): boolean {
  return typeof window !== 'undefined' && !!(window as any)?.go?.gui?.App;
}

// Fallback mock data for browser preview/development
const mockDistro: DistroInfo = {
  id: 'cachyos',
  name: 'CachyOS',
  pretty_name: 'CachyOS (Arch)',
  family: 'arch',
};

const mockStorage: SystemStorageInfo = {
  root: {
    path: '/',
    total_bytes: 100 * 1024 * 1024 * 1024,
    free_bytes: 29.56 * 1024 * 1024 * 1024,
    used_bytes: 70.44 * 1024 * 1024 * 1024,
    used_percent: 70.4,
  },
  home: {
    path: '/home',
    total_bytes: 451.42 * 1024 * 1024 * 1024,
    free_bytes: 202.39 * 1024 * 1024 * 1024,
    used_bytes: 249.03 * 1024 * 1024 * 1024,
    used_percent: 55.2,
  },
  has_separate_home: true,
};

const mockModules: ModuleSummary[] = [
  {
    id: 'dev_cache',
    name: 'Developer Build Caches',
    category: 'Developer Caches',
    description: 'npm, pnpm, Go and cargo package build caches',
    available: true,
    requires_elevation: false,
    enabled: true,
  },
  {
    id: 'pacman',
    name: 'Pacman & AUR',
    category: 'Package Managers',
    description: 'Arch Linux package cache and unneeded dependencies',
    available: true,
    requires_elevation: true,
    enabled: true,
  },
  {
    id: 'flatpak',
    name: 'Flatpak Packages',
    category: 'Package Managers',
    description: 'Unused flatpak runtimes and residual dependencies',
    available: true,
    requires_elevation: false,
    enabled: true,
  },
  {
    id: 'docker',
    name: 'Docker Engine',
    category: 'Containers & System',
    description: 'Stopped containers, dangling images and build cache',
    available: true,
    requires_elevation: false,
    enabled: true,
  },
  {
    id: 'browser_cache',
    name: 'Desktop & Browser Caches',
    category: 'Containers & System',
    description: 'Chromium, Firefox and desktop user thumbnails',
    available: true,
    requires_elevation: false,
    enabled: true,
  },
  {
    id: 'snapper',
    name: 'Btrfs & Snapper',
    category: 'Containers & System',
    description: 'Automated Snapper pre/post snapshot safeguards',
    available: true,
    requires_elevation: true,
    enabled: true,
  },
  {
    id: 'custom',
    name: 'Custom Paths & Rules',
    category: 'Custom Rules',
    description: 'User-defined glob pattern cleaning targets',
    available: true,
    requires_elevation: false,
    enabled: true,
  },
];

const mockScanResults: Record<string, ScanResult> = {
  dev_cache: {
    total_size: 12.26 * 1024 * 1024 * 1024,
    items: [
      {
        id: 'dev:npm-cache',
        name: 'npm Global Cache',
        description: '15,158 cached packages',
        path: '/home/efzyn/.npm/_cacache',
        size: 7.0 * 1024 * 1024 * 1024,
        selected: true,
        safety: 'SAFE',
        metadata: { storage_target: 'home', module: 'dev_cache' },
      },
      {
        id: 'dev:pnpm-store',
        name: 'pnpm Virtual Store',
        description: '138,847 hardlinked package files',
        path: '/home/efzyn/.local/share/pnpm/store',
        size: 3.68 * 1024 * 1024 * 1024,
        selected: true,
        safety: 'SAFE',
        metadata: { storage_target: 'home', module: 'dev_cache' },
      },
      {
        id: 'dev:go-build',
        name: 'Go Build Cache',
        description: '11,240 compilation test & link artifacts',
        path: '/home/efzyn/.cache/go-build',
        size: 1.47 * 1024 * 1024 * 1024,
        selected: true,
        safety: 'SAFE',
        metadata: { storage_target: 'home', module: 'dev_cache' },
      },
      {
        id: 'dev:go-mod',
        name: 'Go Module Download Cache',
        description: 'Downloaded source tarballs & modules',
        path: '/home/efzyn/go/pkg/mod/cache',
        size: 103.36 * 1024 * 1024,
        selected: false,
        safety: 'CAUTION',
        metadata: { storage_target: 'home', module: 'dev_cache' },
      },
    ],
  },
  pacman: {
    total_size: 11.37 * 1024 * 1024,
    items: [
      {
        id: 'pacman:pkg-cache',
        name: 'Pacman Package Cache',
        description: 'Candidate uninstalled archive packages',
        path: '/var/cache/pacman/pkg',
        size: 11.37 * 1024 * 1024,
        selected: true,
        safety: 'SAFE',
        metadata: { storage_target: 'root', module: 'pacman' },
      },
    ],
  },
  flatpak: {
    total_size: 72.62 * 1024 * 1024,
    items: [
      {
        id: 'flatpak:unused-runtime',
        name: 'Unused Flatpak Runtimes',
        description: 'Freed unreferenced GNOME / KDE runtime libraries',
        path: '/var/lib/flatpak/runtime',
        size: 72.62 * 1024 * 1024,
        selected: true,
        safety: 'SAFE',
        metadata: { storage_target: 'root', module: 'flatpak' },
      },
    ],
  },
  docker: {
    total_size: 210.0 * 1024 * 1024,
    items: [
      {
        id: 'docker:dangling-images',
        name: 'Docker Dangling Images',
        description: 'Untagged intermediate build layers',
        path: '/var/lib/docker/overlay2',
        size: 210.0 * 1024 * 1024,
        selected: false,
        safety: 'CAUTION',
        metadata: { storage_target: 'root', module: 'docker' },
      },
    ],
  },
  browser_cache: {
    total_size: 1.55 * 1024 * 1024 * 1024,
    items: [
      {
        id: 'browser:chromium-cache',
        name: 'Chromium Browser Cache',
        description: 'Web HTTP assets and shader caches',
        path: '/home/efzyn/.cache/chromium/Default/Cache',
        size: 1.55 * 1024 * 1024 * 1024,
        selected: true,
        safety: 'SAFE',
        metadata: { storage_target: 'home', module: 'browser_cache' },
      },
    ],
  },
  snapper: {
    total_size: 0,
    items: [],
  },
  custom: {
    total_size: 0,
    items: [],
  },
};

export const api = {
  async getStorageInfo(): Promise<SystemStorageInfo> {
    if (isWailsRuntime()) {
      return (window as any).go.gui.App.GetStorageInfo();
    }
    return Promise.resolve(mockStorage);
  },

  async getDistroInfo(): Promise<DistroInfo> {
    if (isWailsRuntime()) {
      return (window as any).go.gui.App.GetDistroInfo();
    }
    return Promise.resolve(mockDistro);
  },

  async getModules(): Promise<ModuleSummary[]> {
    if (isWailsRuntime()) {
      return (window as any).go.gui.App.GetModules();
    }
    return Promise.resolve(mockModules);
  },

  async scanAll(): Promise<Record<string, ScanResult>> {
    if (isWailsRuntime()) {
      return (window as any).go.gui.App.ScanAll();
    }
    return Promise.resolve(JSON.parse(JSON.stringify(mockScanResults)));
  },

  async scanModule(id: string): Promise<ScanResult> {
    if (isWailsRuntime()) {
      return (window as any).go.gui.App.ScanModule(id);
    }
    return Promise.resolve(mockScanResults[id] || { items: [], total_size: 0 });
  },

  async cleanItems(
    items: CleanableItem[],
    dryRun: boolean,
    snapshotSafeguard: boolean
  ): Promise<CleanResult> {
    if (isWailsRuntime()) {
      return (window as any).go.gui.App.CleanItems(items, dryRun, snapshotSafeguard);
    }
    const freed = items.reduce((acc, it) => acc + (it.size || 0), 0);
    return Promise.resolve({
      cleaned_items: items,
      freed_bytes: freed,
      errors: [],
      duration: 842000000, // 842ms in nanoseconds
    });
  },

  async addCustomRule(
    name: string,
    path: string,
    pattern: string,
    recursive: boolean,
    safety: string
  ): Promise<void> {
    if (isWailsRuntime()) {
      return (window as any).go.gui.App.AddCustomRule(name, path, pattern, recursive, safety);
    }
    return Promise.resolve();
  },

  async removeCustomRule(id: string): Promise<void> {
    if (isWailsRuntime()) {
      return (window as any).go.gui.App.RemoveCustomRule(id);
    }
    return Promise.resolve();
  },
};
