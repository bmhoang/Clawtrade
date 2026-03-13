// Terminal UI Dashboard - renders a multi-panel terminal interface
// Uses ANSI escape codes for rendering (no external TUI library)

export interface DashboardPanel {
  id: string;
  title: string;
  x: number; // column position (0-based)
  y: number; // row position
  width: number; // in columns
  height: number; // in rows
  content: string[];
  border?: boolean;
  focused?: boolean;
}

export interface DashboardLayout {
  panels: DashboardPanel[];
  width: number;
  height: number;
}

// Unicode box-drawing characters
const BOX = {
  single: {
    topLeft: "\u250c",
    topRight: "\u2510",
    bottomLeft: "\u2514",
    bottomRight: "\u2518",
    horizontal: "\u2500",
    vertical: "\u2502",
  },
  double: {
    topLeft: "\u2554",
    topRight: "\u2557",
    bottomLeft: "\u255a",
    bottomRight: "\u255d",
    horizontal: "\u2550",
    vertical: "\u2551",
  },
};

export class TerminalDashboard {
  private panels: Map<string, DashboardPanel> = new Map();
  private width: number;
  private height: number;
  private activePanel: string = "";
  private running: boolean = false;

  constructor(width?: number, height?: number) {
    this.width = width || process.stdout.columns || 120;
    this.height = height || process.stdout.rows || 40;
  }

  /** Add a panel to the dashboard */
  addPanel(panel: DashboardPanel): void {
    this.panels.set(panel.id, { ...panel });
    if (!this.activePanel) {
      this.activePanel = panel.id;
    }
  }

  /** Remove a panel */
  removePanel(id: string): void {
    this.panels.delete(id);
    if (this.activePanel === id) {
      const keys = [...this.panels.keys()];
      this.activePanel = keys.length > 0 ? keys[0] : "";
    }
  }

  /** Update panel content */
  updateContent(id: string, content: string[]): void {
    const panel = this.panels.get(id);
    if (panel) {
      panel.content = content;
    }
  }

  /** Set focused panel */
  setFocus(id: string): void {
    // Unfocus all panels
    for (const panel of this.panels.values()) {
      panel.focused = false;
    }
    const panel = this.panels.get(id);
    if (panel) {
      panel.focused = true;
      this.activePanel = id;
    }
  }

  /** Render the entire dashboard to a string (for testing) */
  render(): string {
    // Build a 2D character buffer filled with spaces
    const buffer: string[][] = [];
    for (let row = 0; row < this.height; row++) {
      buffer.push(new Array(this.width).fill(" "));
    }

    // Draw each panel
    for (const panel of this.panels.values()) {
      if (panel.border !== false) {
        this.drawBox(buffer, panel);
      }
      this.fillContent(buffer, panel);
    }

    // Convert buffer to string
    return buffer.map((row) => row.join("")).join("\n");
  }

  /** Draw a box/border around a panel area */
  private drawBox(buffer: string[][], panel: DashboardPanel): void {
    const chars = panel.focused ? BOX.double : BOX.single;
    const { x, y, width, height } = panel;

    // Clamp drawing to buffer bounds
    const maxRow = Math.min(y + height, this.height);
    const maxCol = Math.min(x + width, this.width);

    // Top border
    if (y < this.height) {
      if (x < this.width) buffer[y][x] = chars.topLeft;
      for (let col = x + 1; col < maxCol - 1; col++) {
        buffer[y][col] = chars.horizontal;
      }
      if (maxCol - 1 > x && maxCol - 1 < this.width)
        buffer[y][maxCol - 1] = chars.topRight;
    }

    // Bottom border
    const bottomRow = y + height - 1;
    if (bottomRow < this.height && bottomRow > y) {
      if (x < this.width) buffer[bottomRow][x] = chars.bottomLeft;
      for (let col = x + 1; col < maxCol - 1; col++) {
        buffer[bottomRow][col] = chars.horizontal;
      }
      if (maxCol - 1 > x && maxCol - 1 < this.width)
        buffer[bottomRow][maxCol - 1] = chars.bottomRight;
    }

    // Side borders
    for (let row = y + 1; row < maxRow - 1; row++) {
      if (x < this.width) buffer[row][x] = chars.vertical;
      if (maxCol - 1 < this.width) buffer[row][maxCol - 1] = chars.vertical;
    }

    // Title in top border
    if (panel.title && y < this.height) {
      const titleStr = ` ${panel.title} `;
      const titleStart = x + 2;
      for (let i = 0; i < titleStr.length; i++) {
        const col = titleStart + i;
        if (col < maxCol - 1 && col < this.width) {
          buffer[y][col] = titleStr[i];
        }
      }
    }
  }

  /** Fill panel content into the buffer */
  private fillContent(buffer: string[][], panel: DashboardPanel): void {
    const hasBorder = panel.border !== false;
    const contentX = panel.x + (hasBorder ? 1 : 0);
    const contentY = panel.y + (hasBorder ? 1 : 0);
    const contentWidth = panel.width - (hasBorder ? 2 : 0);
    const contentHeight = panel.height - (hasBorder ? 2 : 0);

    for (let i = 0; i < Math.min(panel.content.length, contentHeight); i++) {
      const row = contentY + i;
      if (row >= this.height) break;
      const line = panel.content[i];
      for (let j = 0; j < Math.min(line.length, contentWidth); j++) {
        const col = contentX + j;
        if (col >= this.width) break;
        buffer[row][col] = line[j];
      }
    }
  }

  /** Create default trading dashboard layout */
  static createDefaultLayout(
    width: number,
    height: number
  ): DashboardPanel[] {
    const statusBarHeight = 3;
    const mainHeight = height - statusBarHeight;
    const topHeight = Math.floor(mainHeight * 0.43);
    const bottomHeight = mainHeight - topHeight;
    const leftWidth = Math.floor(width * 0.4);
    const rightWidth = width - leftWidth;

    return [
      {
        id: "portfolio",
        title: "Portfolio Summary",
        x: 0,
        y: 0,
        width: leftWidth,
        height: topHeight,
        content: [],
        border: true,
        focused: false,
      },
      {
        id: "chart",
        title: "Price Chart",
        x: leftWidth,
        y: 0,
        width: rightWidth,
        height: topHeight,
        content: [],
        border: true,
        focused: false,
      },
      {
        id: "positions",
        title: "Positions",
        x: 0,
        y: topHeight,
        width: leftWidth,
        height: bottomHeight,
        content: [],
        border: true,
        focused: false,
      },
      {
        id: "chat",
        title: "AI Chat",
        x: leftWidth,
        y: topHeight,
        width: rightWidth,
        height: bottomHeight,
        content: [],
        border: true,
        focused: false,
      },
      {
        id: "status",
        title: "Status",
        x: 0,
        y: mainHeight,
        width: width,
        height: statusBarHeight,
        content: [],
        border: true,
        focused: false,
      },
    ];
  }

  /** Get panel by id */
  getPanel(id: string): DashboardPanel | undefined {
    return this.panels.get(id);
  }

  /** List all panel ids */
  listPanels(): string[] {
    return [...this.panels.keys()];
  }

  /** Get current dimensions */
  getDimensions(): { width: number; height: number } {
    return { width: this.width, height: this.height };
  }
}

// ANSI helpers
export const ansi = {
  moveTo: (x: number, y: number) => `\x1b[${y + 1};${x + 1}H`,
  clear: () => "\x1b[2J",
  reset: () => "\x1b[0m",
  bold: (text: string) => `\x1b[1m${text}\x1b[0m`,
  green: (text: string) => `\x1b[32m${text}\x1b[0m`,
  red: (text: string) => `\x1b[31m${text}\x1b[0m`,
  yellow: (text: string) => `\x1b[33m${text}\x1b[0m`,
  blue: (text: string) => `\x1b[34m${text}\x1b[0m`,
  dim: (text: string) => `\x1b[2m${text}\x1b[0m`,
  hideCursor: () => "\x1b[?25l",
  showCursor: () => "\x1b[?25h",
};
