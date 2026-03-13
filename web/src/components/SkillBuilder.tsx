import { useState, useCallback } from 'react'

// --- Types ---

interface SkillNode {
  id: string
  type: 'trigger' | 'condition' | 'action' | 'output'
  label: string
  config: Record<string, unknown>
  position: { x: number; y: number }
}

interface SkillConnection {
  from: string
  to: string
  label?: string
}

interface SkillBlueprint {
  name: string
  description: string
  nodes: SkillNode[]
  connections: SkillConnection[]
}

// --- Palette definitions ---

interface PaletteEntry {
  type: SkillNode['type']
  label: string
  key: string
}

const paletteGroups: { category: string; entries: PaletteEntry[] }[] = [
  {
    category: 'Triggers',
    entries: [
      { type: 'trigger', label: 'Price Alert', key: 'price_alert' },
      { type: 'trigger', label: 'Schedule', key: 'schedule' },
      { type: 'trigger', label: 'Event', key: 'event' },
    ],
  },
  {
    category: 'Conditions',
    entries: [
      { type: 'condition', label: 'If / Then', key: 'if_then' },
      { type: 'condition', label: 'Compare', key: 'compare' },
      { type: 'condition', label: 'Threshold', key: 'threshold' },
    ],
  },
  {
    category: 'Actions',
    entries: [
      { type: 'action', label: 'Place Order', key: 'place_order' },
      { type: 'action', label: 'Send Alert', key: 'send_alert' },
      { type: 'action', label: 'Call AI', key: 'call_ai' },
    ],
  },
  {
    category: 'Output',
    entries: [
      { type: 'output', label: 'Log', key: 'log' },
      { type: 'output', label: 'Notify', key: 'notify' },
      { type: 'output', label: 'Webhook', key: 'webhook' },
    ],
  },
]

const typeColors: Record<SkillNode['type'], string> = {
  trigger: 'bg-green-600',
  condition: 'bg-yellow-600',
  action: 'bg-blue-600',
  output: 'bg-purple-600',
}

const typeBorders: Record<SkillNode['type'], string> = {
  trigger: 'border-green-500',
  condition: 'border-yellow-500',
  action: 'border-blue-500',
  output: 'border-purple-500',
}

// --- Sub-components ---

function NodePalette({ onAddNode }: { onAddNode: (entry: PaletteEntry) => void }) {
  return (
    <div className="w-56 shrink-0 bg-slate-800 border-r border-slate-700 p-4 overflow-y-auto">
      <h2 className="text-sm font-semibold text-slate-300 uppercase tracking-wider mb-4">Node Palette</h2>
      {paletteGroups.map((group) => (
        <div key={group.category} className="mb-4">
          <h3 className="text-xs font-medium text-slate-400 uppercase mb-2">{group.category}</h3>
          <div className="space-y-1">
            {group.entries.map((entry) => (
              <button
                key={entry.key}
                onClick={() => onAddNode(entry)}
                className={`w-full text-left px-3 py-2 rounded text-sm text-white ${typeColors[entry.type]} hover:opacity-80 transition-opacity`}
              >
                {entry.label}
              </button>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function SkillCanvas({
  nodes,
  connections,
  selectedNodeId,
  onSelectNode,
  connectingFrom,
  onStartConnect,
}: {
  nodes: SkillNode[]
  connections: SkillConnection[]
  selectedNodeId: string | null
  onSelectNode: (id: string) => void
  connectingFrom: string | null
  onStartConnect: (id: string) => void
}) {
  return (
    <div className="flex-1 flex flex-col overflow-hidden">
      {/* Canvas area */}
      <div className="flex-1 relative bg-slate-900 overflow-auto p-4">
        {nodes.length === 0 && (
          <div className="absolute inset-0 flex items-center justify-center text-slate-500 text-sm">
            Add nodes from the palette to get started
          </div>
        )}
        <div className="relative min-h-[600px] min-w-[800px]">
          {nodes.map((node) => (
            <div
              key={node.id}
              onClick={() => onSelectNode(node.id)}
              className={`absolute w-48 rounded-lg border shadow-lg cursor-pointer transition-shadow ${
                typeBorders[node.type]
              } ${selectedNodeId === node.id ? 'ring-2 ring-white shadow-xl' : 'shadow-md'} bg-slate-800`}
              style={{ left: node.position.x, top: node.position.y }}
            >
              <div className={`px-3 py-1.5 rounded-t-lg text-xs font-semibold uppercase text-white ${typeColors[node.type]}`}>
                {node.type}
              </div>
              <div className="px-3 py-2 text-sm text-slate-200">{node.label}</div>
              <div className="px-3 pb-2">
                <button
                  onClick={(e) => {
                    e.stopPropagation()
                    onStartConnect(node.id)
                  }}
                  className={`text-xs px-2 py-0.5 rounded ${
                    connectingFrom === node.id
                      ? 'bg-amber-500 text-black'
                      : 'bg-slate-700 text-slate-300 hover:bg-slate-600'
                  }`}
                >
                  {connectingFrom === node.id ? 'Click target...' : 'Connect'}
                </button>
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Connection list */}
      {connections.length > 0 && (
        <div className="bg-slate-800 border-t border-slate-700 p-3 max-h-36 overflow-y-auto">
          <h3 className="text-xs font-semibold text-slate-400 uppercase mb-2">Connections</h3>
          <div className="space-y-1">
            {connections.map((conn, i) => {
              const fromNode = nodes.find((n) => n.id === conn.from)
              const toNode = nodes.find((n) => n.id === conn.to)
              return (
                <div key={i} className="flex items-center gap-2 text-xs text-slate-300">
                  <span className="font-medium">{fromNode?.label ?? conn.from}</span>
                  <span className="text-slate-500">&rarr;</span>
                  <span className="font-medium">{toNode?.label ?? conn.to}</span>
                  {conn.label && <span className="text-slate-500">({conn.label})</span>}
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}

function NodeEditor({
  node,
  onUpdateNode,
  onRemoveNode,
}: {
  node: SkillNode
  onUpdateNode: (id: string, updates: Partial<Pick<SkillNode, 'label' | 'config'>>) => void
  onRemoveNode: (id: string) => void
}) {
  const configKeys = Object.keys(node.config)

  return (
    <div className="w-64 shrink-0 bg-slate-800 border-l border-slate-700 p-4 overflow-y-auto">
      <h2 className="text-sm font-semibold text-slate-300 uppercase tracking-wider mb-4">Node Editor</h2>

      <label className="block text-xs text-slate-400 mb-1">Label</label>
      <input
        type="text"
        value={node.label}
        onChange={(e) => onUpdateNode(node.id, { label: e.target.value })}
        className="w-full bg-slate-700 border border-slate-600 rounded px-2 py-1 text-sm text-white mb-4"
      />

      <label className="block text-xs text-slate-400 mb-1">Type</label>
      <div className={`inline-block px-2 py-0.5 rounded text-xs text-white mb-4 ${typeColors[node.type]}`}>
        {node.type}
      </div>

      <div className="mb-4">
        <label className="block text-xs text-slate-400 mb-1">Config Fields</label>
        {configKeys.map((key) => (
          <div key={key} className="mb-2">
            <label className="block text-xs text-slate-500">{key}</label>
            <input
              type="text"
              value={String(node.config[key] ?? '')}
              onChange={(e) =>
                onUpdateNode(node.id, {
                  config: { ...node.config, [key]: e.target.value },
                })
              }
              className="w-full bg-slate-700 border border-slate-600 rounded px-2 py-1 text-xs text-white"
            />
          </div>
        ))}
        <AddConfigField
          onAdd={(key) =>
            onUpdateNode(node.id, { config: { ...node.config, [key]: '' } })
          }
        />
      </div>

      <button
        onClick={() => onRemoveNode(node.id)}
        className="w-full px-3 py-1.5 bg-red-600 hover:bg-red-700 rounded text-sm text-white transition-colors"
      >
        Remove Node
      </button>
    </div>
  )
}

function AddConfigField({ onAdd }: { onAdd: (key: string) => void }) {
  const [newKey, setNewKey] = useState('')

  const handleAdd = () => {
    const trimmed = newKey.trim()
    if (trimmed) {
      onAdd(trimmed)
      setNewKey('')
    }
  }

  return (
    <div className="flex gap-1 mt-2">
      <input
        type="text"
        value={newKey}
        onChange={(e) => setNewKey(e.target.value)}
        placeholder="New field name"
        className="flex-1 bg-slate-700 border border-slate-600 rounded px-2 py-1 text-xs text-white"
        onKeyDown={(e) => {
          if (e.key === 'Enter') handleAdd()
        }}
      />
      <button
        onClick={handleAdd}
        className="px-2 py-1 bg-slate-600 hover:bg-slate-500 rounded text-xs text-white"
      >
        +
      </button>
    </div>
  )
}

// --- Main component ---

let nodeIdCounter = 0

export default function SkillBuilder() {
  const [nodes, setNodes] = useState<SkillNode[]>([])
  const [connections, setConnections] = useState<SkillConnection[]>([])
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [blueprintName, setBlueprintName] = useState('Untitled Skill')
  const [blueprintDescription, setBlueprintDescription] = useState('')
  const [connectingFrom, setConnectingFrom] = useState<string | null>(null)

  const addNode = useCallback((entry: PaletteEntry) => {
    const id = `node_${++nodeIdCounter}`
    const newNode: SkillNode = {
      id,
      type: entry.type,
      label: entry.label,
      config: {},
      position: {
        x: 80 + (nodeIdCounter % 4) * 200,
        y: 40 + Math.floor(nodeIdCounter / 4) * 140,
      },
    }
    setNodes((prev) => [...prev, newNode])
    setSelectedNodeId(id)
  }, [])

  const removeNode = useCallback((id: string) => {
    setNodes((prev) => prev.filter((n) => n.id !== id))
    setConnections((prev) => prev.filter((c) => c.from !== id && c.to !== id))
    setSelectedNodeId((prev) => (prev === id ? null : prev))
  }, [])

  const updateNode = useCallback((id: string, updates: Partial<Pick<SkillNode, 'label' | 'config'>>) => {
    setNodes((prev) =>
      prev.map((n) => (n.id === id ? { ...n, ...updates } : n))
    )
  }, [])

  const handleSelectNode = useCallback(
    (id: string) => {
      if (connectingFrom !== null && connectingFrom !== id) {
        // Complete connection
        setConnections((prev) => {
          const exists = prev.some((c) => c.from === connectingFrom && c.to === id)
          if (exists) return prev
          return [...prev, { from: connectingFrom, to: id }]
        })
        setConnectingFrom(null)
      }
      setSelectedNodeId(id)
    },
    [connectingFrom]
  )

  const handleStartConnect = useCallback((id: string) => {
    setConnectingFrom((prev) => (prev === id ? null : id))
  }, [])

  const exportBlueprint = useCallback(() => {
    const blueprint: SkillBlueprint = {
      name: blueprintName,
      description: blueprintDescription,
      nodes,
      connections,
    }
    const json = JSON.stringify(blueprint, null, 2)
    const blob = new Blob([json], { type: 'application/json' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = `${blueprintName.replace(/\s+/g, '_').toLowerCase()}.json`
    a.click()
    URL.revokeObjectURL(url)
  }, [blueprintName, blueprintDescription, nodes, connections])

  const selectedNode = nodes.find((n) => n.id === selectedNodeId) ?? null

  return (
    <div className="flex flex-col h-full bg-slate-900 text-white">
      {/* Toolbar */}
      <div className="flex items-center gap-4 bg-slate-800 border-b border-slate-700 px-4 py-2">
        <h1 className="text-sm font-bold text-slate-300 uppercase tracking-wider shrink-0">Skill Builder</h1>
        <input
          type="text"
          value={blueprintName}
          onChange={(e) => setBlueprintName(e.target.value)}
          className="bg-slate-700 border border-slate-600 rounded px-3 py-1 text-sm text-white w-48"
          placeholder="Blueprint name"
        />
        <input
          type="text"
          value={blueprintDescription}
          onChange={(e) => setBlueprintDescription(e.target.value)}
          className="bg-slate-700 border border-slate-600 rounded px-3 py-1 text-sm text-white flex-1"
          placeholder="Description"
        />
        <span className="text-xs text-slate-500 shrink-0">
          {nodes.length} nodes / {connections.length} connections
        </span>
        <button
          onClick={exportBlueprint}
          className="px-4 py-1.5 bg-emerald-600 hover:bg-emerald-700 rounded text-sm font-medium transition-colors shrink-0"
        >
          Export as JSON
        </button>
      </div>

      {/* Main area */}
      <div className="flex flex-1 overflow-hidden">
        <NodePalette onAddNode={addNode} />
        <SkillCanvas
          nodes={nodes}
          connections={connections}
          selectedNodeId={selectedNodeId}
          onSelectNode={handleSelectNode}
          connectingFrom={connectingFrom}
          onStartConnect={handleStartConnect}
        />
        {selectedNode && (
          <NodeEditor
            node={selectedNode}
            onUpdateNode={updateNode}
            onRemoveNode={removeNode}
          />
        )}
      </div>
    </div>
  )
}
