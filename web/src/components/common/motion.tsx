import type { HTMLMotionProps } from 'motion/react'
import { motion } from 'motion/react'

const easeOut = [0.16, 1, 0.3, 1] as const

const gentleTransition = {
  duration: 0.2,
  ease: easeOut,
}

/**
 * 页面级轻量入场动画。
 * 用于路由页面或大块内容切换，避免每个页面重复声明 motion 参数。
 */
export function PageMotion(props: HTMLMotionProps<'div'>) {
  return (
    <motion.div
      animate={{ opacity: 1, y: 0 }}
      exit={{ opacity: 0, y: 6 }}
      initial={{ opacity: 0, y: 8 }}
      transition={gentleTransition}
      {...props}
    />
  )
}

/**
 * 带子项错峰入场的列表容器。
 * 与 MotionItem 成对使用，适合资源行列表；表格大量行或虚拟列表不要使用逐项动画。
 */
export function MotionList(props: HTMLMotionProps<'div'>) {
  return (
    <motion.div
      animate="show"
      initial="hidden"
      variants={{
        hidden: {},
        show: {
          transition: {
            staggerChildren: 0.035,
          },
        },
      }}
      {...props}
    />
  )
}

/**
 * MotionList 的单个子项动画包装。
 * 用于卡片行、设置项等少量可感知列表项；不要包裹会频繁重排的复杂输入控件。
 */
export function MotionItem(props: HTMLMotionProps<'div'>) {
  return (
    <motion.div
      variants={{
        hidden: { opacity: 0, y: 8 },
        show: { opacity: 1, y: 0, transition: gentleTransition },
      }}
      {...props}
    />
  )
}
