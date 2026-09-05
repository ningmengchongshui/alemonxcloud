import classNames from 'classnames'

export function XCoinMark({
  className,
  title = 'XCoin'
}: {
  className?: string
  title?: string
}) {
  return (
    <svg
      className={classNames(
        'inline-block size-[1em] shrink-0 align-[-0.125em]',
        className
      )}
      viewBox="0 0 24 24"
      fill="none"
      role="img"
      aria-label={title}
    >
      <path
        d="M4 3.5c4.2 2.3 7.2 5.4 9.8 9.2 2.1 3.1 3.7 5.5 4.5 7.8"
        stroke="currentColor"
        strokeWidth="2.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
      <path
        d="M20 4.5c-3.7-.7-6.7.8-9.2 3.4-3.1 3.1-4.4 7-7.5 10.4"
        stroke="currentColor"
        strokeWidth="2.6"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </svg>
  )
}
