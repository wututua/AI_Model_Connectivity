import { useState } from 'react'
import type { ProbeOptions } from '../../types'
import { Input } from '../../components/ui/input'
import { Textarea } from '../../components/ui/textarea'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '../../components/ui/select'
import { Switch } from '../../components/ui/switch'
import { Field } from './shared'

export const defaultProbe: ProbeOptions = {
  capability: 'text', assert_contains: '', assert_json: false, assert_json_keys: '',
  protocol: 'chat', stream: false, max_tokens: 16, token_limit_field: 'max_tokens',
  omit_temperature: false, temperature: 0, timeout_seconds: 0, prompt: '', system_prompt: '', omit_system_prompt: false,
}

export function ProbeFields({ value, onChange }: { value: ProbeOptions; onChange: (value: ProbeOptions) => void }) {
  const [preset, setPreset] = useState('custom')
  const set = <K extends keyof ProbeOptions>(key: K, next: ProbeOptions[K]) => { setPreset('custom'); onChange({ ...value, [key]: next }) }
  const applyPreset = (name: string) => {
    setPreset(name)
    if (name === 'custom') return
    onChange({
      ...value, ...defaultProbe, prompt: value.prompt, system_prompt: value.system_prompt,
      protocol: name === 'responses' ? 'responses' : 'chat',
      max_tokens: name === 'reasoning' || name === 'responses' ? 1024 : 16,
      token_limit_field: name === 'reasoning' ? 'max_completion_tokens' : 'max_tokens',
      omit_temperature: name === 'reasoning' || name === 'responses',
      stream: name === 'stream',
    })
  }
  return <section className="min-w-0 space-y-4 border-t pt-4 md:col-span-2" aria-label="探测配置">
    <h3 className="text-sm font-semibold">探测配置</h3>
    <div className="field-grid">
      <Field label="检测能力"><Select value={value.capability || 'text'} onValueChange={capability => onChange({ ...value, capability, ...(capability !== 'text' ? { protocol: 'chat', stream: false } : {}) })}><SelectTrigger aria-label="检测能力"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="text">文本响应</SelectItem><SelectItem value="tools">工具调用结构</SelectItem><SelectItem value="embedding">Embedding 向量</SelectItem></SelectContent></Select></Field>
      <div />
      <Field label="兼容预设"><Select value={preset} onValueChange={applyPreset}><SelectTrigger aria-label="兼容预设"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="custom">自定义</SelectItem><SelectItem value="legacy">Chat 轻量文本</SelectItem><SelectItem value="reasoning">Chat 推理参数</SelectItem><SelectItem value="responses">Responses 文本</SelectItem><SelectItem value="stream">Chat 流式文本</SelectItem></SelectContent></Select></Field>
      <Field label="探测协议"><Select disabled={!!value.capability && value.capability !== 'text'} value={value.protocol || 'chat'} onValueChange={next => set('protocol', next)}><SelectTrigger aria-label="探测协议"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="chat">Chat Completions</SelectItem><SelectItem value="responses">Responses</SelectItem></SelectContent></Select></Field>
      <Field label="输出 Token 上限" htmlFor="probe-max"><Input id="probe-max" type="number" min={1} max={131072} value={value.max_tokens || 16} onChange={e => set('max_tokens', Number(e.target.value))} /></Field>
      <Field label="超时（秒，0 为全局值）" htmlFor="probe-timeout"><Input id="probe-timeout" type="number" min={0} max={86400} step="any" value={value.timeout_seconds} onChange={e => set('timeout_seconds', Number(e.target.value))} /></Field>
      {value.protocol !== 'responses' && <Field label="Token 参数"><Select value={value.token_limit_field || 'max_tokens'} onValueChange={next => set('token_limit_field', next)}><SelectTrigger aria-label="Token 参数"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="max_tokens">max_tokens</SelectItem><SelectItem value="max_completion_tokens">max_completion_tokens</SelectItem></SelectContent></Select></Field>}
      <Field label="Temperature" htmlFor="probe-temperature"><Input id="probe-temperature" type="number" min={0} max={2} step="0.1" value={value.temperature} disabled={value.omit_temperature} onChange={e => set('temperature', Number(e.target.value))} /></Field>
      <label className="flex items-center justify-between gap-3 text-sm">发送 temperature<Switch checked={!value.omit_temperature} onCheckedChange={next => set('omit_temperature', !next)} aria-label="发送 temperature" /></label>
      <label className="flex items-center justify-between gap-3 text-sm">流式探测<Switch disabled={!!value.capability && value.capability !== 'text'} checked={value.stream} onCheckedChange={next => set('stream', next)} aria-label="流式探测" /></label>
      <Field label="用户提示词" htmlFor="probe-prompt" className="md:col-span-2"><Textarea id="probe-prompt" maxLength={4096} value={value.prompt} placeholder="ping" onChange={e => set('prompt', e.target.value)} /></Field>
      <label className="flex items-center justify-between gap-3 text-sm md:col-span-2">发送系统提示词<Switch checked={!value.omit_system_prompt} onCheckedChange={next => set('omit_system_prompt', !next)} aria-label="发送系统提示词" /></label>
      {!value.omit_system_prompt && <Field label="系统提示词" htmlFor="probe-system" className="md:col-span-2"><Textarea id="probe-system" maxLength={4096} value={value.system_prompt} placeholder="No thinking. Respond only with exactly: pang. No extra words." onChange={e => set('system_prompt', e.target.value)} /></Field>}
      {(!value.capability || value.capability === 'text') && <>
        <Field label="必须包含的文本"><Input aria-label="必须包含的文本" maxLength={4096} value={value.assert_contains || ''} onChange={e => set('assert_contains', e.target.value)} /></Field>
        <label className="flex items-center justify-between gap-3 text-sm">要求 JSON 对象<Switch checked={!!value.assert_json} onCheckedChange={next => set('assert_json', next)} aria-label="要求 JSON 对象" /></label>
        <Field label="必需 JSON 字段（逗号分隔）"><Input aria-label="必需 JSON 字段" maxLength={1024} value={value.assert_json_keys || ''} onChange={e => set('assert_json_keys', e.target.value)} /></Field>
      </>}
    </div>
  </section>
}
