import { useState, type InputHTMLAttributes } from 'react'
import { Eye, EyeOff } from 'lucide-react'
import { Input } from './ui/input'
import { Button } from './ui/button'

export function PasswordInput(props: InputHTMLAttributes<HTMLInputElement>) {
  const [show, setShow] = useState(false)
  return <div className="relative">
    <Input {...props} type={show ? 'text' : 'password'} className="pr-10" />
    <Button type="button" variant="ghost" size="icon" className="absolute right-0 top-0" onClick={() => setShow(value => !value)} aria-label={show ? '隐藏密码' : '显示密码'} title={show ? '隐藏密码' : '显示密码'}><span>{show ? <EyeOff /> : <Eye />}</span></Button>
  </div>
}
