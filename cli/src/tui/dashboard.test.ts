import { describe, expect, test } from "bun:test";
import {
  TerminalDashboard,
  ansi,
  type DashboardPanel,
} from "./dashboard";

describe("TerminalDashboard", () => {
  function makePanel(overrides: Partial<DashboardPanel> = {}): DashboardPanel {
    return {
      id: "test",
      title: "Test",
      x: 0,
      y: 0,
      width: 20,
      height: 5,
      content: [],
      border: true,
      focused: false,
      ...overrides,
    };
  }

  test("addPanel adds panel to dashboard", () => {
    const dash = new TerminalDashboard(80, 24);
    dash.addPanel(makePanel({ id: "p1" }));
    expect(dash.getPanel("p1")).toBeDefined();
    expect(dash.listPanels()).toContain("p1");
  });

  test("removePanel removes panel", () => {
    const dash = new TerminalDashboard(80, 24);
    dash.addPanel(makePanel({ id: "p1" }));
    dash.addPanel(makePanel({ id: "p2" }));
    dash.removePanel("p1");
    expect(dash.getPanel("p1")).toBeUndefined();
    expect(dash.listPanels()).not.toContain("p1");
    expect(dash.listPanels()).toContain("p2");
  });

  test("updateContent updates panel content", () => {
    const dash = new TerminalDashboard(80, 24);
    dash.addPanel(makePanel({ id: "p1" }));
    dash.updateContent("p1", ["line 1", "line 2"]);
    const panel = dash.getPanel("p1");
    expect(panel?.content).toEqual(["line 1", "line 2"]);
  });

  test("setFocus changes active panel", () => {
    const dash = new TerminalDashboard(80, 24);
    dash.addPanel(makePanel({ id: "p1" }));
    dash.addPanel(makePanel({ id: "p2", x: 20 }));
    dash.setFocus("p2");
    expect(dash.getPanel("p2")?.focused).toBe(true);
    expect(dash.getPanel("p1")?.focused).toBe(false);
  });

  test("render produces output string", () => {
    const dash = new TerminalDashboard(40, 10);
    dash.addPanel(makePanel({ id: "p1", width: 20, height: 5 }));
    const output = dash.render();
    expect(typeof output).toBe("string");
    expect(output.length).toBeGreaterThan(0);
  });

  test("render includes panel borders with box-drawing chars", () => {
    const dash = new TerminalDashboard(40, 10);
    dash.addPanel(makePanel({ id: "p1", width: 20, height: 5 }));
    const output = dash.render();
    // Single-line box drawing characters for unfocused panels
    expect(output).toContain("\u250c"); // top-left corner
    expect(output).toContain("\u2510"); // top-right corner
    expect(output).toContain("\u2514"); // bottom-left corner
    expect(output).toContain("\u2518"); // bottom-right corner
    expect(output).toContain("\u2500"); // horizontal
    expect(output).toContain("\u2502"); // vertical
  });

  test("render uses double-line borders for focused panel", () => {
    const dash = new TerminalDashboard(40, 10);
    dash.addPanel(makePanel({ id: "p1", width: 20, height: 5, focused: true }));
    const output = dash.render();
    expect(output).toContain("\u2554"); // double top-left
    expect(output).toContain("\u2557"); // double top-right
    expect(output).toContain("\u255a"); // double bottom-left
    expect(output).toContain("\u255d"); // double bottom-right
    expect(output).toContain("\u2550"); // double horizontal
    expect(output).toContain("\u2551"); // double vertical
  });

  test("render includes panel titles", () => {
    const dash = new TerminalDashboard(40, 10);
    dash.addPanel(makePanel({ id: "p1", title: "MyPanel", width: 20, height: 5 }));
    const output = dash.render();
    expect(output).toContain("MyPanel");
  });

  test("render includes panel content", () => {
    const dash = new TerminalDashboard(40, 10);
    dash.addPanel(
      makePanel({
        id: "p1",
        width: 20,
        height: 5,
        content: ["Hello World"],
      })
    );
    const output = dash.render();
    expect(output).toContain("Hello World");
  });

  test("createDefaultLayout creates 5 panels", () => {
    const panels = TerminalDashboard.createDefaultLayout(120, 40);
    expect(panels).toHaveLength(5);
  });

  test("createDefaultLayout panels cover full width", () => {
    const width = 120;
    const panels = TerminalDashboard.createDefaultLayout(width, 40);

    // Top row panels should span full width
    const topPanels = panels.filter((p) => p.y === 0);
    expect(topPanels).toHaveLength(2);
    const topLeftWidth = topPanels.find((p) => p.x === 0)!.width;
    const topRight = topPanels.find((p) => p.x > 0)!;
    expect(topLeftWidth + topRight.width).toBe(width);

    // Status bar spans full width
    const statusBar = panels.find((p) => p.id === "status")!;
    expect(statusBar.width).toBe(width);
  });

  test("createDefaultLayout has expected panel ids", () => {
    const panels = TerminalDashboard.createDefaultLayout(120, 40);
    const ids = panels.map((p) => p.id);
    expect(ids).toContain("portfolio");
    expect(ids).toContain("chart");
    expect(ids).toContain("positions");
    expect(ids).toContain("chat");
    expect(ids).toContain("status");
  });

  test("getPanel returns correct panel", () => {
    const dash = new TerminalDashboard(80, 24);
    const p = makePanel({ id: "myPanel", title: "My Panel" });
    dash.addPanel(p);
    const retrieved = dash.getPanel("myPanel");
    expect(retrieved).toBeDefined();
    expect(retrieved?.title).toBe("My Panel");
    expect(retrieved?.id).toBe("myPanel");
  });

  test("getPanel returns undefined for missing panel", () => {
    const dash = new TerminalDashboard(80, 24);
    expect(dash.getPanel("nonexistent")).toBeUndefined();
  });

  test("listPanels returns all ids", () => {
    const dash = new TerminalDashboard(80, 24);
    dash.addPanel(makePanel({ id: "a" }));
    dash.addPanel(makePanel({ id: "b", x: 20 }));
    dash.addPanel(makePanel({ id: "c", x: 40 }));
    const ids = dash.listPanels();
    expect(ids).toHaveLength(3);
    expect(ids).toContain("a");
    expect(ids).toContain("b");
    expect(ids).toContain("c");
  });

  test("getDimensions returns correct size", () => {
    const dash = new TerminalDashboard(100, 50);
    const dims = dash.getDimensions();
    expect(dims.width).toBe(100);
    expect(dims.height).toBe(50);
  });
});

describe("ANSI helpers", () => {
  test("moveTo produces correct escape code", () => {
    // moveTo(0,0) should be ESC[1;1H (1-indexed)
    expect(ansi.moveTo(0, 0)).toBe("\x1b[1;1H");
    expect(ansi.moveTo(5, 10)).toBe("\x1b[11;6H");
  });

  test("clear produces correct escape code", () => {
    expect(ansi.clear()).toBe("\x1b[2J");
  });

  test("reset produces correct escape code", () => {
    expect(ansi.reset()).toBe("\x1b[0m");
  });

  test("bold wraps text with bold codes", () => {
    const result = ansi.bold("hello");
    expect(result).toBe("\x1b[1mhello\x1b[0m");
  });

  test("green wraps text with green color codes", () => {
    const result = ansi.green("ok");
    expect(result).toBe("\x1b[32mok\x1b[0m");
  });

  test("red wraps text with red color codes", () => {
    const result = ansi.red("err");
    expect(result).toBe("\x1b[31merr\x1b[0m");
  });

  test("yellow wraps text with yellow color codes", () => {
    const result = ansi.yellow("warn");
    expect(result).toBe("\x1b[33mwarn\x1b[0m");
  });

  test("blue wraps text with blue color codes", () => {
    const result = ansi.blue("info");
    expect(result).toBe("\x1b[34minfo\x1b[0m");
  });

  test("dim wraps text with dim codes", () => {
    const result = ansi.dim("faded");
    expect(result).toBe("\x1b[2mfaded\x1b[0m");
  });

  test("hideCursor produces correct escape code", () => {
    expect(ansi.hideCursor()).toBe("\x1b[?25l");
  });

  test("showCursor produces correct escape code", () => {
    expect(ansi.showCursor()).toBe("\x1b[?25h");
  });
});
