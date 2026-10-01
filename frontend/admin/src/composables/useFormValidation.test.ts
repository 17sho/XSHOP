import { describe, expect, it } from 'vitest'
import { rules, useFormValidation } from './useFormValidation'

describe('useFormValidation.validateField', () => {
  it('returns true and clears the error when all rules pass', () => {
    const validation = useFormValidation({ name: [rules.required()] })

    expect(validation.validateField('name', '')).toBe(false)
    expect(validation.errors.name).toBe('This field is required')
    expect(validation.validateField('name', 'valid')).toBe(true)
    expect(validation.errors.name).toBe('')
  })

  it('treats a field without rules as valid', () => {
    const validation = useFormValidation({})
    expect(validation.validateField('unknown', 'anything')).toBe(true)
  })
})
